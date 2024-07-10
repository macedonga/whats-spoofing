# whats-spoofing

Reply spoofer for WhatsApp messages.

## Installation

```bash
$ git clone https://github.com/macedonga/whats-spoofing.git
$ cd whats-spoofing
$ mkdir -p data && mkdir -p db && mkdir -p history
$ go mod download
$ go build
```

## Usage

```bash
$ ./whats-spoofing
```

Once you see a QR code on the terminal, scan it with your WhatsApp app just like when you login on WhatsApp web. \
This is necessary to get the session token to send messages. \
After that, when you see on your terminal `Listening on localhost:8080`, open a browser tab and go to `http://localhost:8080`.

You will see a form to send messages. \
Fill the form with the phone number (with the international prefix but without +) you want to send the message to, the number of the user to spoof (the number that will appear on the message), the spoofed reply message and the reply, sent by you. \
If you want to send a spoofed message to a group, just copy and paste the group ID (which you can get below the sender form) on the first phone number input. \
Click on the `Send` button and the message will be sent.

Happy spoofing! 🎉

**Use this tool responsibly! I'm not responsible for any misuse of this tool.**

---

*I forked and updated this project with a newer version of Whatsmeow and a web interface.* \
*The original project has been deleted from Github, but it was made by [@lichti](https://github.com/lichti/)*