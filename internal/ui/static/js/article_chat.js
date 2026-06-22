// Article chat assistant: a toggled chat bubble on the entry page that lets the
// user ask questions about the article and related topics. Conversation state
// lives in the browser; each request sends the full history to the server.
function initializeArticleChat() {
    const container = document.getElementById("article-chat");
    if (!container) {
        return;
    }

    const chatURL = container.dataset.chatUrl;
    const toggle = document.getElementById("article-chat-toggle");
    const panel = document.getElementById("article-chat-panel");
    const closeButton = document.getElementById("article-chat-close");
    const log = document.getElementById("article-chat-log");
    const form = document.getElementById("article-chat-form");
    const input = document.getElementById("article-chat-input");
    const sendButton = form ? form.querySelector(".article-chat-send") : null;

    if (!toggle || !panel || !log || !form || !input) {
        return;
    }

    const messages = [];
    let busy = false;
    let greeted = false;

    function appendMessage(role, text) {
        const element = document.createElement("div");
        element.className = "article-chat-msg article-chat-msg-" + role;
        element.textContent = text;
        log.appendChild(element);
        log.scrollTop = log.scrollHeight;
        return element;
    }

    function openPanel() {
        panel.hidden = false;
        toggle.setAttribute("aria-expanded", "true");
        if (!greeted) {
            appendMessage("assistant", "Hi! Ask me anything about this article or related topics. I can search the web and read the full article when needed.");
            greeted = true;
        }
        input.focus();
    }

    function closePanel() {
        panel.hidden = true;
        toggle.setAttribute("aria-expanded", "false");
    }

    toggle.addEventListener("click", () => {
        if (panel.hidden) {
            openPanel();
        } else {
            closePanel();
        }
    });

    if (closeButton) {
        closeButton.addEventListener("click", closePanel);
    }

    function setBusy(value) {
        busy = value;
        input.disabled = value;
        if (sendButton) {
            sendButton.disabled = value;
        }
    }

    async function send(question) {
        appendMessage("user", question);
        messages.push({ role: "user", content: question });

        const pending = appendMessage("assistant", "…");
        setBusy(true);

        try {
            const response = await fetch(chatURL, {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                    "X-Csrf-Token": document.body.dataset.csrfToken || "",
                },
                body: JSON.stringify({ messages: messages }),
            });

            if (!response.ok) {
                const detail = await response.text();
                pending.className = "article-chat-msg article-chat-msg-error";
                pending.textContent = "Error: " + (detail || response.statusText);
                return;
            }

            const data = await response.json();
            const reply = data.reply || "(no answer)";
            pending.textContent = reply;
            messages.push({ role: "assistant", content: reply });
        } catch (error) {
            pending.className = "article-chat-msg article-chat-msg-error";
            pending.textContent = "Error: " + error;
        } finally {
            setBusy(false);
            input.focus();
        }
    }

    form.addEventListener("submit", (event) => {
        event.preventDefault();
        if (busy) {
            return;
        }
        const question = input.value.trim();
        if (!question) {
            return;
        }
        input.value = "";
        send(question);
    });
}

initializeArticleChat();
