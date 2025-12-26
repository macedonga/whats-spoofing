# whats-spoofing

Reply spoofer for WhatsApp messages.

## Download

Latest nightly builds:
- [Windows (exe)](https://github.com/macedonga/whats-spoofing/releases/download/nightly/whats-spoofing-windows.exe)
- [Linux](https://github.com/macedonga/whats-spoofing/releases/download/nightly/whats-spoofing-linux)

## Building

```bash
$ git clone https://github.com/macedonga/whats-spoofing.git
$ cd whats-spoofing
$ go mod download
$ go build
```

## Usage

```bash
$ ./whats-spoofing # or .\whats-spoofing.exe on Windows
```

After running the command, if ran for the first time, a "whats-spoofing.db" file will be created in the current directory. \
This file contains the WhatsApp session data and must be kept safe.

Open your browser and go to `http://localhost:8080/`. You'll have to scan the QR code with your WhatsApp app, just like when you login on WhatsApp web. \
After scanning the QR code, the page will reload and you'll see some inputs and a list of group IDs.

Fill the form with the phone number (with the international prefix but without +) you want to send the message to, the number of the user to spoof (the number that will appear on the message), the spoofed reply message and the reply, sent by you. \
If you want to send a spoofed message to a group, just copy and paste the group ID (which you can get below the sender form) on the first phone number input. \
Click on the `Send` button and the message will be sent.

Happy spoofing! 🎉

> [!CAUTION]
> Use this tool responsibly! I'm not responsible for any misuse of this tool.