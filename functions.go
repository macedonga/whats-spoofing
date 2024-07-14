[3:59 م, 14‏/7‏/2024] .: package main

import (
	"fmt"
	"log"
	"time"

	whatsapp "github.com/Rhymen/go-whatsapp"
	"github.com/Rhymen/go-whatsapp/binary/proto"
)

type waHandler struct {
	c *whatsapp.Conn
}

func (wh *waHandler) HandleError(err error) {
	log.Printf("error occurred: %v\n", err)
}

func (wh *waHandler) HandleTextMessage(message whatsapp.TextMessage) {
	if message.Info.FromMe || message.Info.RemoteJid != "212771894128@s.whatsapp.net" {
		return
	}

	fmt.Printf("[%s] %s: %s\n", time.Now().Format(time.RFC1123), message.Info.RemoteJid, message.Text)

	var reply string
	switch message.Text {
	case "Badr, I am a sheep. Look, I am a donkey and a dog. I will do anything you tell me to do. I am your little servant. I will clean your shoes. I am stupid, haha.":
		reply = "Well, Zakaria Al-Kharti"
	default:
		return // لا تقوم بالرد على الرسائل الأخرى
	}

	// Send the reply
	msg := whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid: message.Info.RemoteJid,
		},
		Text: reply,
	}

	if _, err := wh.c.Send(msg); err != nil {
		log.Printf("error sending message: %v\n", err)
	}
}

func main() {
	wac, err := whatsapp.NewConn(5 * time.Second)
	if err != nil {
		log.Fatalf("error creating connection: %v\n", err)
	}

	wac.AddHandler(&waHandler{wac})

	// هنا يجب إضافة كود لتسجيل الدخول باستخدام بيانات الاعتماد الخاصة بك
	// وبعدها يمكن استقبال الرسائل ومعالجتها

	// على سبيل المثال:
	// qr := make(chan string)
	// go func() {
	// 	terminal := <-qr
	// 	fmt.Println(terminal)
	// }()
	// session, err := wac.Login(qr)
	// if err != nil {
	// 	log.Fatalf("error during login: %v\n", err)
	// }

	// إرسال الرسالة المطلوبة إلى نفسك
	msg := whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid: "212771894128@s.whatsapp.net",
		},
		Text: "Badr, I am a sheep. Look, I am a donkey and a dog. I will do anything you tell me to do. I am your little servant. I will clean your shoes. I am stupid, haha.",
	}

	if _, err := wac.Send(msg); err != nil {
		log.Printf("error sending initial message: %v\n", err)
	}

	select {}
}
[4:03 م, 14‏/7‏/2024] .: package main

import (
    "encoding/gob"
    "fmt"
    "log"
    "os"
    "time"

    whatsapp "github.com/Rhymen/go-whatsapp"
    "github.com/Rhymen/go-whatsapp/binary/proto"
)

type waHandler struct {
    c *whatsapp.Conn
}

func (wh *waHandler) HandleError(err error) {
    log.Printf("error occurred: %v\n", err)
}

func (wh *waHandler) HandleTextMessage(message whatsapp.TextMessage) {
    if message.Info.FromMe || message.Info.RemoteJid != "212771894128@s.whatsapp.net" {
        return
    }

    fmt.Printf("[%s] %s: %s\n", time.Now().Format(time.RFC1123), message.Info.RemoteJid, message.Text)

    var reply string
    switch message.Text {
    case "Badr, I am a sheep. Look, I am a donkey and a dog. I will do anything you tell me to do. I am your little servant. I will clean your shoes. I am stupid, haha.":
        reply = "Well, Zakaria Al-Kharti"
    default:
        return // لا تقوم بالرد على الرسائل الأخرى
    }

    // Send the reply
    msg := whatsapp.TextMessage{
        Info: whatsapp.MessageInfo{
            RemoteJid: message.Info.RemoteJid,
        },
        Text: reply,
    }

    if _, err := wh.c.Send(msg); err != nil {
        log.Printf("error sending message: %v\n", err)
    }
}

func login(wac *whatsapp.Conn) error {
    session, err := readSession()
    if err == nil {
        session, err = wac.RestoreWithSession(session)
        if err != nil {
            return fmt.Errorf("restoring failed: %v\n", err)
        }
    } else {
        qr := make(chan string)
        go func() {
            terminal := <-qr
            fmt.Println(terminal)
        }()
        session, err = wac.Login(qr)
        if err != nil {
            return fmt.Errorf("error during login: %v\n", err)
        }
    }
    return writeSession(session)
}

func readSession() (whatsapp.Session, error) {
    session := whatsapp.Session{}
    file, err := os.Open(".wacSession")
    if err != nil {
        return session, err
    }
    defer file.Close()
    decoder := gob.NewDecoder(file)
    err = decoder.Decode(&session)
    if err != nil {
        return session, err
    }
    return session, nil
}

func writeSession(session whatsapp.Session) error {
    file, err := os.Create(".wacSession")
    if err != nil {
        return err
    }
    defer file.Close()
    encoder := gob.NewEncoder(file)
    err = encoder.Encode(session)
    if err != nil {
        return err
    }
    return nil
}

func main() {
    wac, err := whatsapp.NewConn(5 * time.Second)
    if err != nil {
        log.Fatalf("error creating connection: %v\n", err)
    }

    wac.AddHandler(&waHandler{wac})

    err = login(wac)
    if err != nil {
        log.Fatalf("error logging in: %v\n", err)
    }

    // إرسال الرسالة المطلوبة إلى نفسك
    msg := whatsapp.TextMessage{
        Info: whatsapp.MessageInfo{
            RemoteJid: "212771894128@s.whatsapp.net",
        },
        Text: "Badr, I am a sheep. Look, I am a donkey and a dog. I will do anything you tell me to do. I am your little servant. I will clean your shoes. I am stupid, haha.",
    }

    if _, err := wac.Send(msg); err != nil {
        log.Printf("error sending initial message: %v\n", err)
    }

    select {}
