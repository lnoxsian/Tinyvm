// TinyVM client-side script
document.addEventListener("DOMContentLoaded", () => {
    // Theme toggle helper if configured
    const savedTheme = localStorage.getItem("tinyvm-theme");
    if (savedTheme) {
        document.documentElement.setAttribute("data-theme", savedTheme);
    }

    // HTMX response error handler
    document.body.addEventListener("htmx:responseError", (evt) => {
        let msg = "Action failed (HTTP " + evt.detail.xhr.status + ")";
        try {
            const res = JSON.parse(evt.detail.xhr.responseText);
            if (res.error && res.error.message) {
                msg = res.error.message;
            }
        } catch (e) {
            if (evt.detail.xhr.responseText && evt.detail.xhr.responseText.length < 200) {
                msg = evt.detail.xhr.responseText;
            }
        }
        alert(msg);
    });
});
