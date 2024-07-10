const fetchGids = async () => {
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

    container.innerHTML = data.join("");
};

fetchGids();

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
        cid += "@s.whatsapp.net";

    let data = {
        "chat_id": cid,
        "spoofed_id": sid,
        "message_id": "!",
        "spoofed_message": spf_msg,
        "reply_message": rpl_msg
    };

    const res = await fetch("/send-spoofed", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(data)
    });

    const text = await res.json();

    alert(text.message);
};