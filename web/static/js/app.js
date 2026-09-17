// TinyVM vanilla JS helpers
document.addEventListener("DOMContentLoaded", () => {
    // Theme toggle helper if configured
    const savedTheme = localStorage.getItem("tinyvm-theme");
    if (savedTheme) {
        document.documentElement.setAttribute("data-theme", savedTheme);
    }
});
