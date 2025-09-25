let statusCheckInterval;

const showScreen = (screenId) => {
    document.getElementById("loading-screen").classList.add("hidden");
    document.getElementById("qr-screen").classList.add("hidden");
    document.getElementById("main-screen").classList.add("hidden");
    
    document.getElementById(screenId).classList.remove("hidden");
};

const checkStatus = async () => {
    try {
        const res = await fetch("/status");
        const status = await res.json();
        
        if (status.loggedIn) {
            // User is logged in, show main interface
            clearInterval(statusCheckInterval);
            showScreen("main-screen");
            fetchGids();
        } else {
            // User is not logged in, show QR screen
            showScreen("qr-screen");
            
            if (status.qr) {
                // Display the QR code
                displayQRCode(status.qr);
            } else {
                // Show waiting message if no QR code yet
                const img = document.getElementById("qr-image");
                img.src = "data:image/svg+xml;base64," + btoa(`
                    <svg width="256" height="256" xmlns="http://www.w3.org/2000/svg">
                        <rect width="256" height="256" fill="#f3f4f6"/>
                        <text x="128" y="128" font-family="Inter, sans-serif" font-size="16" 
                              text-anchor="middle" fill="#6b7280">Generating QR Code...</text>
                    </svg>
                `);
                img.alt = "Generating QR Code...";
            }
            
            // Start polling for status changes
            if (!statusCheckInterval) {
                statusCheckInterval = setInterval(checkStatus, 3000);
            }
        }
    } catch (error) {
        console.error("Failed to check status:", error);
        showScreen("loading-screen");
        setTimeout(checkStatus, 2000);
    }
};

const displayQRCode = (qrData) => {
    const img = document.getElementById("qr-image");
    const qrUrl = `https://api.qrserver.com/v1/create-qr-code/?size=256x256&data=${encodeURIComponent(qrData)}`;
    img.src = qrUrl;
    img.alt = "WhatsApp QR Code";
};

const fetchGids = async () => {
    try {
        const res = await fetch("/get-groups");
        let data = (await res.text()).split("\n").map(e => e.trim());

        data = data
            .filter(e => e.length > 0)
            .map(e => {
                let split = e.split(":");
                const id = split.pop();
                const name = split.join(":");

                return `<div class="group-container"><b>${name}</b><br /><code>${id}</code></div>`;
            });

        const container = document.getElementById("gids");
        
        if (data.length === 0) {
            container.innerHTML = '<div class="text-gray-400 text-center">No groups found</div>';
        } else {
            container.innerHTML = data.join("");
        }
    } catch (error) {
        console.error("Failed to fetch groups:", error);
        document.getElementById("gids").innerHTML = '<div class="text-red-400 text-center">Failed to load groups</div>';
    }
};

const sendForm = async () => {
    const form = document.querySelectorAll("form")[0];

    let cid = form.elements["cid"].value;
    let sid = form.elements["sid"].value;
    let spf_msg = form.elements["spf_msg"].value;
    let rpl_msg = form.elements["rpl_msg"].value;

    if (cid.length === 0 || sid.length === 0 || spf_msg.length === 0 || rpl_msg.length === 0) {
        alert("All fields are required");
        return;
    }

    if (!cid.includes("@"))
        cid += "@s.whatsapp.net";
    
    if (!sid.includes("@"))
        sid += "@s.whatsapp.net";

    let data = {
        "chat_id": cid,
        "spoofed_id": sid,
        "message_id": "!",
        "spoofed_message": spf_msg,
        "reply_message": rpl_msg
    };

    try {
        const res = await fetch("/send-spoofed", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(data)
        });

        const text = await res.json();
        alert(text.message);
        
        // Clear form after successful send
        form.reset();
    } catch (error) {
        console.error("Failed to send message:", error);
        alert("Failed to send message. Please try again.");
    }
};

// Start checking status when page loads
checkStatus();