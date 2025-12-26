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

Download the latest release or build the project as shown above. Then, run the executable by double-clicking it or via command line:

```bash
$ ./whats-spoofing-linux # On Windows use `whats-spoofing-windows.exe`
```

After running the command, if ran for the first time, a "whats-spoofing.db" file will be created in the current directory. \
This file contains the WhatsApp session data and must be kept safe.

Open your browser and go to `http://localhost:8080/`. You'll have to scan the QR code with your WhatsApp app, just like when you login on WhatsApp web. \
After scanning the QR code, the page will reload and you'll see some inputs.

You can click on the input fields and select a contact or group from the list that appears. \
If you want you can also manually type the JID (phone number with international prefix but without +) of the contact or group ID you want to send the spoofed message to. \
After that, you can insert the message you want your spoofed message to respond to and the spoofed reply message.

Click on the `Send` button and the message will be sent.

Happy spoofing! 🎉

> [!CAUTION]
> Use this tool responsibly! I'm not responsible for any misuse of this tool.