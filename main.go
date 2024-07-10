package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var cli *whatsmeow.Client
var log waLog.Logger

var logLevel = "INFO"
var debugLogs = flag.Bool("debug", false, "Enable debug logs?")
var dbDialect = flag.String("db-dialect", "sqlite3", "Database dialect (sqlite3 or postgres)")
var dbAddress = flag.String("db-address", "file:db/whatsbot.db?_foreign_keys=on", "Database address")
var requestFullSync = flag.Bool("request-full-sync", false, "Request full (1 year) history sync when logging in?")
var mediaPath = flag.String("media-path", "media", "Path to store media files in")
var historyPath = flag.String("history-path", "history", "Path to store history files in")
var pairRejectChan = make(chan bool, 1)

var getIDSecret string

func main() {
	waBinary.IndentXML = true
	flag.Parse()

	if *debugLogs {
		logLevel = "DEBUG"
	}
	if *requestFullSync {
		store.DeviceProps.RequireFullSync = proto.Bool(true)
	}
	log = waLog.Stdout("Main", logLevel, true)

	dbLog := waLog.Stdout("Database", logLevel, true)
	storeContainer, err := sqlstore.New(*dbDialect, *dbAddress, dbLog)
	if err != nil {
		log.Errorf("Failed to connect to database: %v", err)
		return
	}
	device, err := storeContainer.GetFirstDevice()
	if err != nil {
		log.Errorf("Failed to get device: %v", err)
		return
	}

	cli = whatsmeow.NewClient(device, waLog.Stdout("Client", logLevel, true))
	getIDSecret = cli.GenerateMessageID()
	var isWaitingForPair atomic.Bool
	cli.PrePairCallback = func(jid types.JID, platform, businessName string) bool {
		isWaitingForPair.Store(true)
		defer isWaitingForPair.Store(false)
		log.Infof("Pairing %s (platform: %q, business name: %q). Type r within 3 seconds to reject pair", jid, platform, businessName)
		select {
		case reject := <-pairRejectChan:
			if reject {
				log.Infof("Rejecting pair")
				return false
			}
		case <-time.After(3 * time.Second):
		}
		log.Infof("Accepting pair")
		return true
	}

	ch, err := cli.GetQRChannel(context.Background())
	if err != nil {
		// This error means that we're already logged in, so ignore it.
		if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			log.Errorf("Failed to get QR channel: %v", err)
		}
	} else {
		go func() {
			for evt := range ch {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				} else {
					log.Infof("QR channel result: %s", evt.Event)
				}
			}
		}()
	}

	cli.AddEventHandler(handler)
	err = cli.Connect()
	if err != nil {
		log.Errorf("Failed to connect: %v", err)
		return
	}

	c := make(chan os.Signal, 1)
	input := make(chan string)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer close(input)
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			line := strings.TrimSpace(scan.Text())
			if len(line) > 0 {
				input <- line
			}
		}
	}()
	for {
		select {
		case <-c:
			log.Infof("Interrupt received, exiting")
			cli.Disconnect()
			return
		case cmd := <-input:
			if len(cmd) == 0 {
				log.Infof("Stdin closed, exiting")
				cli.Disconnect()
				return
			}
			if isWaitingForPair.Load() {
				if cmd == "r" {
					pairRejectChan <- true
				} else if cmd == "a" {
					pairRejectChan <- false
				}
				continue
			}
			args := strings.Fields(cmd)
			cmd = args[0]
			args = args[1:]
			go handleCmd(strings.ToLower(cmd), args)
		}
	}
}

var historySyncID int32
var startupTime = time.Now().Unix()

func handleCmd(cmd string, args []string) {
	handleCmd1(cmd, args, nil)
}

func handleCmd1(cmd string, args []string, evt *events.Message) (output string) {
	output = "Command not found"
	switch cmd {
	case "getgroup":
		output = cmdGetGroup(args)
		log.Infof("output: %s", output)
	case "listgroups":
		output = cmdListGroups(args)
		log.Infof("output: %s", output)
	case "send-spoofed-reply":
		print("args: ", args)
		output = cmdSendSpoofedReply(args)
		log.Infof("output: %s", output)
	case "send-spoofed-img-reply":
		output = cmdSendSpoofedImgReply(args)
		log.Infof("output: %s", output)
	case "send-spoofed-demo":
		output = cmdSendSpoofedDemo(args)
		log.Infof("output: %s", output)
	case "send-spoofed-demo-img":
		output = cmdSendSpoofedDemoImg(args)
		log.Infof("output: %s", output)
	case "spoofed-reply-this":
		if evt != nil {
			if evt.Message.ExtendedTextMessage != nil {
				if evt.Message.ExtendedTextMessage.ContextInfo != nil {
					if evt.Message.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
						output = cmdSpoofedReplyThis(args, evt.Message)
						log.Infof("output: %s", output)
					}
				}
			} else {
				output = "You need use this command replying a message"
				log.Infof("output: %s", output)
			}
		} else {
			output = "You need to reply a message using your whatsapp client to use this command"
			log.Infof("output: %s", output)
		}
	}
	return
}

type RequestSendSpoofed struct {
	CID     string `json:"chat_id"`
	MID     string `json:"message_id"`
	SID     string `json:"spoofed_id"`
	SPF_MSG string `json:"spoofed_message"`
	RPL_MSG string `json:"reply_message"`
}

type ResponseData struct {
	Message string `json:"message"`
}

func spoofedMsgSenderHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("HTTP Request: %s", r.URL.Path)
	if r.Method == "POST" {
		var requestData RequestSendSpoofed
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			http.Error(w, "Error parsing JSON request body", http.StatusBadRequest)
			return
		}

		if requestData.CID == "" || requestData.MID == "" || requestData.SID == "" || requestData.SPF_MSG == "" || requestData.RPL_MSG == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var args = []string{requestData.CID, requestData.MID, requestData.SID, requestData.SPF_MSG + "|" + requestData.RPL_MSG}

		print("args: ", args)

		var output = cmdSendSpoofedReply(args)
		log.Infof("output: %s", output)

		w.WriteHeader(http.StatusOK)

		w.Header().Set("Content-Type", "application/json")

		responseData := ResponseData{
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

func getGroupsHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("HTTP Request: %s", r.URL.Path)
	if r.Method == "GET" {
		var output = cmdListGroups([]string{})
		log.Infof("output: %s", output)

		w.WriteHeader(http.StatusOK)

		w.Write([]byte(output))

		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Invalid request"))
}

func handler(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		if len(cli.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := cli.SendPresence(types.PresenceAvailable)
			log.Errorf("AppStateSyncComplete %s: %v", rawEvt, err)
		}
		return
	case *events.Connected, *events.PushNameSetting:
		if len(cli.Store.PushName) == 0 {
			return
		}
		err := cli.SendPresence(types.PresenceAvailable)
		log.Errorf("Connected %s: %v", rawEvt, err)

		r := http.NewServeMux()

		buildHandler := http.FileServer(http.Dir("client/"))
		r.Handle("/", buildHandler)
		r.HandleFunc("/send-spoofed", spoofedMsgSenderHandler)
		r.HandleFunc("/get-groups", getGroupsHandler)

		srv := &http.Server{
			Handler:      r,
			Addr:         "127.0.0.1:8080",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		log.Infof("Listening on localhost:8080")
		if srv.ListenAndServe() != nil {
			log.Errorf("Failed to start server: %v", err)
		}

		return
	case *events.StreamReplaced:
		os.Exit(0)
	case *events.Message:
		log.Infof("Received message %s from %s (%s)", evt.Info.ID, evt.Info.SourceString())

		if strings.HasPrefix(getMsg(evt), getIDSecret) {
			if evt.Info.IsFromMe {
				text := fmt.Sprintf("-> Cmd output: \nChatID %v", evt.Info.Chat)
				jid, _ := parseJID(cli.Store.ID.User)
				sendConversationMessage(jid, text)
				return
			}
		}

		if strings.HasPrefix(getMsg(evt), "/setSecrete ") {
			if evt.Info.IsFromMe {
				jid, _ := parseJID(cli.Store.ID.User)
				if evt.Info.Chat.String() == jid.String() {
					words := strings.SplitN(getMsg(evt), " ", 2)
					if len(words) > 1 {
						strWords := words[1]
						getIDSecret = strWords
						text := fmt.Sprintf("-> Cmd output: \nbSecret set to %s", getIDSecret)
						sendConversationMessage(jid, text)
					} else {
						text := fmt.Sprintf("-> Cmd output: \nYou need to set a secret")
						sendConversationMessage(jid, text)
					}
					return
				}
			}
		}

		if strings.HasPrefix(getMsg(evt), "/cmd ") {
			if evt.Info.IsFromMe {
				jid, _ := parseJID(cli.Store.ID.User)
				if evt.Info.Chat.String() == jid.String() {
					words := strings.SplitN(getMsg(evt), " ", 3)
					if len(words) > 1 {
						strCommand := words[1]
						strParameters := ""
						out := handleCmd1(strCommand, []string{}, evt)
						if len(words) > 2 {
							strParameters = words[2]
							out = handleCmd1(strCommand, strings.Split(strParameters, " "), evt)
						}
						text := fmt.Sprintf("-> Cmd output: %s", out)
						sendConversationMessage(jid, text)
					} else {
						text := fmt.Sprintf("-> Cmd output: \nYou need send a valid command")
						sendConversationMessage(jid, text)
					}
					return
				}
			}
		}

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
	log.Infof("Event(%s): \n  File: %s \n  Extra: %+v \n  Raw: %+v", evt_type, file_name, extra, raw)
	return nil
}

func postEvent(evt_type string, raw interface{}, extra interface{}) error {
	log.Infof("Event(%s): \n%+v", evt_type, raw)
	return nil
}

func postError(evt_type string, evt_error string, raw interface{}) error {
	log.Errorf("Error(%s): %s \n%+v", evt_type, evt_error, raw)
	return nil
}
