// TinyVM client-side script

/* ==========================================================================
   TinyVM Integrated In-HTML Popup Message & Modal Dialog System
   ========================================================================== */

const TinyVM = window.TinyVM || {};
window.TinyVM = TinyVM;

// SVG icons for modal dialogs and toasts
const POPUP_ICONS = {
    info: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>`,
    warning: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`,
    danger: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`,
    error: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`,
    success: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`,
    question: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`
};

/**
 * Show an integrated in-HTML modal popup dialog
 * @param {Object} opts Configuration options
 * @returns {Promise<boolean>}
 */
function createPopupModal(opts) {
    return new Promise((resolve) => {
        // Remove any existing modal if open
        const existing = document.getElementById("tvm-modal-backdrop");
        if (existing) {
            existing.remove();
        }

        const type = opts.type || "info";
        const isConfirm = !!opts.isConfirm;
        const title = opts.title || (type === "danger" || type === "error" ? "Error" : type === "warning" ? "Warning" : isConfirm ? "Confirmation Required" : "Notice");
        const message = opts.message || "";
        const confirmText = opts.confirmText || (isConfirm ? "Confirm" : "OK");
        const cancelText = opts.cancelText || "Cancel";
        const confirmClass = opts.confirmClass || (type === "danger" || type === "error" ? "btn-danger-solid" : "btn-primary");
        const iconSvg = POPUP_ICONS[type] || POPUP_ICONS.info;

        const backdrop = document.createElement("div");
        backdrop.id = "tvm-modal-backdrop";
        backdrop.className = "tvm-modal-backdrop";
        backdrop.setAttribute("tabindex", "-1");

        const card = document.createElement("div");
        card.className = `tvm-modal-card tvm-modal-${type}`;
        card.setAttribute("role", "dialog");
        card.setAttribute("aria-modal", "true");
        card.setAttribute("aria-labelledby", "tvm-modal-title");

        // Header
        const header = document.createElement("div");
        header.className = "tvm-modal-header";

        const headerLeft = document.createElement("div");
        headerLeft.className = "tvm-modal-header-left";

        const iconBadge = document.createElement("div");
        iconBadge.className = `tvm-modal-icon-badge tvm-modal-icon-${type}`;
        iconBadge.innerHTML = iconSvg;

        const titleEl = document.createElement("h3");
        titleEl.id = "tvm-modal-title";
        titleEl.className = "tvm-modal-title";
        titleEl.textContent = title;

        headerLeft.appendChild(iconBadge);
        headerLeft.appendChild(titleEl);

        const closeBtn = document.createElement("button");
        closeBtn.type = "button";
        closeBtn.className = "tvm-modal-close";
        closeBtn.setAttribute("aria-label", "Close dialog");
        closeBtn.innerHTML = "&times;";

        header.appendChild(headerLeft);
        header.appendChild(closeBtn);

        // Body
        const body = document.createElement("div");
        body.className = "tvm-modal-body";
        const msgEl = document.createElement("div");
        msgEl.className = "tvm-modal-message";
        msgEl.textContent = message;
        body.appendChild(msgEl);

        // Footer
        const footer = document.createElement("div");
        footer.className = "tvm-modal-footer";

        let cancelBtn = null;
        if (isConfirm) {
            cancelBtn = document.createElement("button");
            cancelBtn.type = "button";
            cancelBtn.className = "btn tvm-modal-btn-cancel";
            cancelBtn.textContent = cancelText;
            footer.appendChild(cancelBtn);
        }

        const confirmBtn = document.createElement("button");
        confirmBtn.type = "button";
        confirmBtn.className = `btn ${confirmClass} tvm-modal-btn-confirm`;
        confirmBtn.textContent = confirmText;
        footer.appendChild(confirmBtn);

        card.appendChild(header);
        card.appendChild(body);
        card.appendChild(footer);
        backdrop.appendChild(card);

        const container = document.getElementById("tvm-modal-root") || document.body;
        container.appendChild(backdrop);

        // Trigger entrance animation
        requestAnimationFrame(() => {
            backdrop.classList.add("tvm-modal-visible");
        });

        // Focus management: If destructive confirm, focus Cancel to avoid accidental Enter key; else focus confirm
        if (isConfirm && (type === "danger" || confirmClass.includes("danger")) && cancelBtn) {
            cancelBtn.focus();
        } else {
            confirmBtn.focus();
        }

        let isClosed = false;
        function closeModal(result) {
            if (isClosed) return;
            isClosed = true;

            window.removeEventListener("keydown", handleKeydown);
            backdrop.classList.remove("tvm-modal-visible");
            setTimeout(() => {
                backdrop.remove();
            }, 180);
            resolve(result);
        }

        function handleKeydown(e) {
            if (e.key === "Escape") {
                e.preventDefault();
                closeModal(false);
            } else if (e.key === "Enter") {
                if (document.activeElement === cancelBtn) {
                    e.preventDefault();
                    closeModal(false);
                } else if (!e.shiftKey && !e.altKey && !e.ctrlKey) {
                    e.preventDefault();
                    closeModal(true);
                }
            }
        }

        window.addEventListener("keydown", handleKeydown);

        closeBtn.addEventListener("click", () => closeModal(false));
        if (cancelBtn) {
            cancelBtn.addEventListener("click", () => closeModal(false));
        }
        confirmBtn.addEventListener("click", () => closeModal(true));

        backdrop.addEventListener("click", (e) => {
            if (e.target === backdrop) {
                closeModal(false);
            }
        });
    });
}

/**
 * Integrated in-HTML Alert popup message
 * Replaces window.alert
 */
TinyVM.alert = function(message, options = {}) {
    if (typeof options === "string") {
        options = { title: options };
    }
    const isError = /error|failed|failure|unable|cannot|invalid/i.test(message);
    const type = options.type || (isError ? "error" : "info");
    const title = options.title || (type === "error" ? "Error" : type === "warning" ? "Warning" : "Notice");

    return createPopupModal({
        message: String(message),
        title: title,
        type: type,
        isConfirm: false,
        confirmText: options.okText || "OK",
        confirmClass: options.confirmClass || "btn-primary"
    });
};

/**
 * Integrated in-HTML Confirmation popup message
 * Replaces window.confirm & powers HTMX hx-confirm
 */
TinyVM.confirm = function(message, options = {}) {
    if (typeof options === "string") {
        options = { title: options };
    }

    const isDestructive = /delete|erase|destroy|remove|stop|reset|rollback|force|power off/i.test(message);
    const type = options.type || (isDestructive ? "danger" : "warning");
    const title = options.title || (isDestructive ? "Confirm Action" : "Confirmation Required");
    const confirmText = options.confirmText || (isDestructive && /delete|erase|remove/i.test(message) ? "Delete" : "Confirm");
    const confirmClass = options.confirmClass || (isDestructive ? "btn-danger-solid" : "btn-primary");

    return createPopupModal({
        message: String(message),
        title: title,
        type: type,
        isConfirm: true,
        confirmText: confirmText,
        cancelText: options.cancelText || "Cancel",
        confirmClass: confirmClass
    });
};

// Floating toast notification system
function showToast(message, type = "success", duration = 3500) {
    let container = document.getElementById("toast-container");
    if (!container) {
        container = document.createElement("div");
        container.id = "toast-container";
        container.className = "toast-container";
        document.body.appendChild(container);
    }

    const toast = document.createElement("div");
    toast.className = `toast toast-${type}`;

    let iconSvg = "";
    if (type === "success") {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`;
    } else if (type === "error" || type === "danger") {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`;
    } else if (type === "warning") {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="#d29922" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`;
    } else {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="#58a6ff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>`;
    }

    const textSpan = document.createElement("span");
    textSpan.textContent = message;

    const closeBtn = document.createElement("button");
    closeBtn.type = "button";
    closeBtn.className = "toast-close";
    closeBtn.setAttribute("aria-label", "Dismiss notification");
    closeBtn.innerHTML = "&times;";

    toast.innerHTML = iconSvg;
    toast.appendChild(textSpan);
    toast.appendChild(closeBtn);
    container.appendChild(toast);

    requestAnimationFrame(() => toast.classList.add("show"));

    let dismissTimer;
    function dismiss() {
        clearTimeout(dismissTimer);
        toast.classList.remove("show");
        setTimeout(() => toast.remove(), 250);
    }

    closeBtn.addEventListener("click", dismiss);
    if (duration > 0) {
        dismissTimer = setTimeout(dismiss, duration);
    }
}

TinyVM.toast = showToast;

// Global helper exports
window.showAlert = TinyVM.alert;
window.showConfirm = TinyVM.confirm;
window.showToast = showToast;

// Override native browser alert with integrated in-HTML popup message!
window.alert = function(msg) {
    return TinyVM.alert(msg);
};

/* ==========================================================================
   Navigation & Reload Helpers
   ========================================================================== */

function safeReload(delay = 0) {
    if (delay > 0) {
        setTimeout(() => {
            window.location.reload();
        }, delay);
    } else {
        window.location.reload();
    }
}

function safeNavigate(url, delay = 0) {
    if (delay > 0) {
        setTimeout(() => {
            window.location.href = url;
        }, delay);
    } else {
        window.location.href = url;
    }
}

window.safeReload = safeReload;
window.safeNavigate = safeNavigate;

function setAutostart(enabled) {
    const input = document.getElementById('autostart-input');
    const group = document.getElementById('autostart-toggle-group');
    if (!input || !group) return;
    input.value = enabled ? 'true' : 'false';
    const onBtn = group.querySelector('.btn-toggle-on');
    const offBtn = group.querySelector('.btn-toggle-off');
    if (onBtn && offBtn) {
        if (enabled) {
            onBtn.classList.add('active');
            offBtn.classList.remove('active');
        } else {
            onBtn.classList.remove('active');
            offBtn.classList.add('active');
        }
    }
}
window.setAutostart = setAutostart;

// Neutralize any beforeunload prompts to ensure smooth navigation & reload
window.addEventListener("beforeunload", (e) => {
    delete e.returnValue;
}, { capture: true });
window.onbeforeunload = null;

// Auto-refresh when VM is in an intermediate transitional state ("stopping" or "starting")
function initVMTransitionWatcher() {
    const badge = document.getElementById("vm-state-badge");
    if (!badge) return;

    const currentState = (badge.getAttribute("data-state") || "").toLowerCase().trim();
    // Strictly only monitor transitional states. NEVER poll when steady ("running" or "stopped").
    if (currentState !== "stopping" && currentState !== "starting") {
        return;
    }

    const vmIdMatch = window.location.pathname.match(/\/vms\/([^/]+)/);
    if (!vmIdMatch) return;
    const vmId = vmIdMatch[1];
    if (vmId === "new") return;

    let reloaded = false;
    const interval = setInterval(async () => {
        if (reloaded) return;
        try {
            const res = await fetch(`/api/v1/vms/${encodeURIComponent(vmId)}`, {
                headers: { "Accept": "application/json" }
            });
            if (!res.ok) return;
            const data = await res.json();
            // TinyVM API returns status in data.status
            const newStatus = (data.status || "").toLowerCase().trim();

            // When the state transitions away from the intermediate state (e.g. stopping -> stopped)
            if (newStatus && newStatus !== currentState) {
                reloaded = true;
                clearInterval(interval);
                safeReload(150);
            }
        } catch (e) {
            // Ignore transient network errors during process teardown
        }
    }, 1000);
}

document.addEventListener("DOMContentLoaded", () => {
    // Watch for state transitions if VM is stopping or starting
    initVMTransitionWatcher();

    // Theme toggle helper if configured
    const savedTheme = localStorage.getItem("tinyvm-theme");
    if (savedTheme) {
        document.documentElement.setAttribute("data-theme", savedTheme);
    }

    // Global HTMX confirm interception: replace native browser confirm with integrated HTML popup
    document.body.addEventListener("htmx:confirm", (evt) => {
        const question = evt.detail.question || (evt.target && evt.target.getAttribute("hx-confirm"));
        if (!question) return;

        // Suppress native browser confirm dialog
        evt.preventDefault();

        TinyVM.confirm(question).then((confirmed) => {
            if (confirmed) {
                // Re-issue HTMX request with prompt skipped
                evt.detail.issueRequest(true);
            }
        });
    });

    // Global form data-confirm interception
    document.body.addEventListener("submit", (evt) => {
        const form = evt.target;
        if (!form || !form.getAttribute) return;
        const confirmMsg = form.getAttribute("data-confirm");
        if (confirmMsg && !form.dataset.tvmConfirmed) {
            evt.preventDefault();
            TinyVM.confirm(confirmMsg).then((confirmed) => {
                if (confirmed) {
                    form.dataset.tvmConfirmed = "true";
                    form.submit();
                    delete form.dataset.tvmConfirmed;
                }
            });
        }
    });

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

        // If error occurred on wizard form, render inside #wizard-error banner
        const wizardErr = document.getElementById("wizard-error");
        const wizardErrText = document.getElementById("wizard-error-text");
        if (wizardErr && wizardErrText) {
            wizardErrText.textContent = msg;
            wizardErr.style.display = "flex";
            wizardErr.scrollIntoView({ behavior: "smooth", block: "center" });
        } else {
            showToast(msg, "error");
        }
    });

    // Handle toast notification events dispatched by HTMX HX-Trigger
    document.body.addEventListener("show-toast", (evt) => {
        const data = evt.detail;
        if (typeof data === "string") {
            showToast(data, "success");
        } else if (data && data.message) {
            showToast(data.message, data.type || "success");
        }
    });

    // ISO updated event handler for dynamic list count & empty state
    document.body.addEventListener("iso-updated", () => {
        showToast("ISO image deleted successfully", "success");
        const tbody = document.getElementById("iso-table-body");
        if (!tbody) return;
        const remaining = tbody.querySelectorAll("tr").length;
        const countMeta = document.getElementById("iso-stat-count");
        if (countMeta) {
            countMeta.textContent = `${remaining} ISO image${remaining === 1 ? '' : 's'} discovered`;
        }
        if (remaining === 0) {
            const cardContainer = document.getElementById("iso-card-container");
            if (cardContainer) {
                cardContainer.outerHTML = `
                    <div class="empty-state card" id="iso-card-container">
                        <h3>No ISO Images Found</h3>
                        <p style="color: var(--text-muted);">Place your installation ISOs in the ISO directory or upload one below to attach them to virtual machines.</p>
                    </div>`;
            }
        }
    });

    // Initialize VM Creation Wizard if present
    initCreateVMWizard();

    // Initialize Storage Page Actions if present
    initStorageActions();

    // Helper to safely schedule telemetry rendering via requestAnimationFrame
    function scheduleTelemetryRender(pushPoint = false) {
        requestAnimationFrame(() => {
            renderVMTelemetryCharts(pushPoint);
        });
    }

    // Initialize VM Telemetry sparklines if present
    scheduleTelemetryRender(true);
    window.addEventListener("resize", () => scheduleTelemetryRender(false));

    // Redraw when window or document becomes visible/focused
    document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === "visible") {
            scheduleTelemetryRender(false);
        }
    });
    window.addEventListener("focus", () => scheduleTelemetryRender(false));

    // Handle HTMX swaps, settles, and loads
    document.addEventListener("htmx:afterSwap", (evt) => {
        if (document.querySelector(".vm-telemetry-canvas")) {
            scheduleTelemetryRender(true);
        }
    });
    document.addEventListener("htmx:afterSettle", (evt) => {
        if (document.querySelector(".vm-telemetry-canvas")) {
            scheduleTelemetryRender(false);
        }
    });
    if (window.htmx) {
        window.htmx.onLoad(() => {
            if (document.querySelector(".vm-telemetry-canvas")) {
                scheduleTelemetryRender(true);
            }
        });
    }

    // Watchdog: Ensure visible canvases in active summary tab are always drawn and never blank
    setInterval(() => {
        const summaryTab = document.getElementById("tab-summary");
        if (summaryTab && summaryTab.classList.contains("active")) {
            const canvases = document.querySelectorAll(".vm-telemetry-canvas");
            if (canvases.length > 0) {
                renderVMTelemetryCharts(false);
            }
        }
    }, 2000);
});

/* ==========================================================================
   VM Creation Wizard Handlers (Phase 8)
   ========================================================================== */

let currentWizardStep = 1;

function initCreateVMWizard() {
    const form = document.getElementById("vm-wizard-form");
    if (!form) return;

    const nameInput = document.getElementById("vm-name");
    const idInput = document.getElementById("vm-id");
    const sshPortInput = document.getElementById("vm-ssh-port");
    const sshPreviewCmd = document.getElementById("ssh-preview-cmd");

    // Auto-derive slug ID from name if user hasn't manually edited ID
    let userEditedID = false;
    if (idInput) {
        idInput.addEventListener("input", () => {
            userEditedID = idInput.value.trim().length > 0;
        });
    }

    if (nameInput && idInput) {
        nameInput.addEventListener("input", () => {
            if (!userEditedID) {
                idInput.value = nameInput.value
                    .toLowerCase()
                    .replace(/[^a-z0-9._-]/g, "-")
                    .replace(/-+/g, "-")
                    .slice(0, 64);
            }
        });
    }

    if (sshPortInput && sshPreviewCmd) {
        sshPortInput.addEventListener("input", () => {
            const port = sshPortInput.value.trim() || "2222";
            sshPreviewCmd.textContent = `ssh user@localhost -p ${port}`;
        });
    }

    // Clear error banner when any input changes
    form.addEventListener("input", () => {
        const wizardErr = document.getElementById("wizard-error");
        if (wizardErr) wizardErr.style.display = "none";
    });
}

function goToStep(stepNumber) {
    if (stepNumber < 1 || stepNumber > 4) return;

    const wizardErr = document.getElementById("wizard-error");
    if (wizardErr) wizardErr.style.display = "none";

    currentWizardStep = stepNumber;

    // Update Stepper Navigation Indicators
    for (let i = 1; i <= 4; i++) {
        const stepEl = document.getElementById(`step-nav-${i}`);
        const panelEl = document.getElementById(`wizard-panel-${i}`);
        const connectorEl = document.getElementById(`step-connector-${i}`);

        if (stepEl) {
            stepEl.classList.remove("active", "completed");
            if (i === stepNumber) {
                stepEl.classList.add("active");
            } else if (i < stepNumber) {
                stepEl.classList.add("completed");
            }
        }

        if (panelEl) {
            panelEl.classList.toggle("active", i === stepNumber);
        }

        if (connectorEl) {
            connectorEl.classList.toggle("completed", i < stepNumber);
        }
    }

    // Populate review card if entering Step 4
    if (stepNumber === 4) {
        populateReviewCard();
    }

    window.scrollTo({ top: 0, behavior: "smooth" });
}

function showWizardError(msg) {
    const wizardErr = document.getElementById("wizard-error");
    const wizardErrText = document.getElementById("wizard-error-text");
    if (wizardErr && wizardErrText) {
        wizardErrText.textContent = msg;
        wizardErr.style.display = "flex";
        wizardErr.scrollIntoView({ behavior: "smooth", block: "center" });
    } else {
        TinyVM.alert(msg, { type: "error", title: "Configuration Error" });
    }
}

function validateAndNext(stepNumber) {
    if (stepNumber === 1) {
        const nameInput = document.getElementById("vm-name");
        if (!nameInput || !nameInput.value.trim()) {
            showWizardError("Please enter a valid VM Name to proceed.");
            if (nameInput) nameInput.focus();
            return;
        }
        const nameRegex = /^[a-zA-Z0-9][a-zA-Z0-9._ -]{0,63}$/;
        if (!nameRegex.test(nameInput.value.trim())) {
            showWizardError("VM Name must start with an alphanumeric character and only contain letters, numbers, dots, hyphens, and spaces.");
            nameInput.focus();
            return;
        }
        goToStep(2);
    } else if (stepNumber === 2) {
        const cpuInput = document.getElementById("cpu-number");
        const ramInput = document.getElementById("ram-number");
        const cpus = parseInt(cpuInput ? cpuInput.value : "0", 10);
        const ram = parseInt(ramInput ? ramInput.value : "0", 10);

        if (isNaN(cpus) || cpus < 1 || cpus > 256) {
            showWizardError("CPU count must be between 1 and 256.");
            if (cpuInput) cpuInput.focus();
            return;
        }
        if (isNaN(ram) || ram < 128) {
            showWizardError("Memory allocation must be at least 128 MB.");
            if (ramInput) ramInput.focus();
            return;
        }
        goToStep(3);
    } else if (stepNumber === 3) {
        const diskSizeInput = document.getElementById("vm-disk-size");
        if (!diskSizeInput || !diskSizeInput.value.trim()) {
            showWizardError("Please enter a virtual disk size (e.g. 20G).");
            if (diskSizeInput) diskSizeInput.focus();
            return;
        }
        const diskRegex = /^[0-9]+[MGTmgt]?$/;
        if (!diskRegex.test(diskSizeInput.value.trim())) {
            showWizardError("Disk size must be a number optionally followed by M, G, or T (e.g. 20G).");
            diskSizeInput.focus();
            return;
        }

        const netEnable = document.getElementById("vm-net-enable");
        if (netEnable && netEnable.checked) {
            const sshInput = document.getElementById("vm-ssh-port");
            if (sshInput && sshInput.value.trim()) {
                const port = parseInt(sshInput.value.trim(), 10);
                if (isNaN(port) || port < 1 || port > 65535) {
                    showWizardError("SSH host port must be between 1 and 65535.");
                    sshInput.focus();
                    return;
                }
            }
        }
        goToStep(4);
    }
}

function syncCPU(val) {
    const slider = document.getElementById("cpu-slider");
    const num = document.getElementById("cpu-number");
    if (slider) slider.value = val;
    if (num) num.value = val;
}

function syncRAM(val) {
    const slider = document.getElementById("ram-slider");
    const num = document.getElementById("ram-number");
    if (slider) slider.value = val;
    if (num) num.value = val;
}

function toggleNetworkFields(checked) {
    const fields = document.getElementById("network-fields");
    if (fields) {
        fields.style.display = checked ? "block" : "none";
    }
}

function onArchChange(arch) {
    const uefiRadio = document.querySelector('input[name="firmware"][value="uefi"]');
    const biosRadio = document.querySelector('input[name="firmware"][value="bios"]');
    const uefiCard = document.getElementById('firmware-card-uefi');

    if (arch === 'x86' || arch === 'i386') {
        if (biosRadio) biosRadio.checked = true;
        if (uefiRadio) {
            uefiRadio.disabled = true;
            if (uefiCard) {
                uefiCard.classList.add('disabled');
                uefiCard.style.opacity = '0.5';
                uefiCard.style.cursor = 'not-allowed';
                uefiCard.title = '32-bit (x86) requires SeaBIOS';
            }
        }
    } else {
        if (uefiRadio && !uefiRadio.hasAttribute("data-host-missing")) {
            uefiRadio.disabled = false;
            if (uefiCard) {
                uefiCard.classList.remove('disabled');
                uefiCard.style.opacity = '1';
                uefiCard.style.cursor = 'pointer';
                uefiCard.title = '';
            }
        }
    }
    updateFirmwareCards();
}

function onOSChange(osVal) {
    const archSelect = document.getElementById("vm-arch");
    if (!archSelect) return;
    const is32 = osVal.includes("32-bit") || osVal.includes("ReactOS") || osVal.includes("FreeDOS");
    if (is32) {
        archSelect.value = "x86";
        onArchChange("x86");
    } else if (osVal !== "Other") {
        archSelect.value = "x86_64";
        onArchChange("x86_64");
    }
}

function updateFirmwareCards() {
    const biosCard = document.getElementById("firmware-card-bios");
    const uefiCard = document.getElementById("firmware-card-uefi");
    const selected = document.querySelector('input[name="firmware"]:checked');
    if (!selected) return;

    if (biosCard) biosCard.classList.toggle("selected", selected.value === "bios");
    if (uefiCard) uefiCard.classList.toggle("selected", selected.value === "uefi");
}

function populateReviewCard() {
    const name = document.getElementById("vm-name")?.value.trim() || "-";
    const id = document.getElementById("vm-id")?.value.trim() || name;
    const arch = document.getElementById("vm-arch")?.value || "x86_64";
    const archLabel = (arch === "x86" || arch === "i386") ? "x86 (32-bit)" : "x86_64 (64-bit)";
    const os = document.getElementById("vm-os")?.value || "Linux";
    const isoSelect = document.getElementById("vm-iso");
    const iso = (isoSelect && isoSelect.value) ? isoSelect.value : "None (No media attached)";
    const firmwareRadio = document.querySelector('input[name="firmware"]:checked');
    const firmware = firmwareRadio ? firmwareRadio.value : "bios";

    const cpus = document.getElementById("cpu-number")?.value || "2";
    const ram = document.getElementById("ram-number")?.value || "2048";
    const ramGB = (parseInt(ram, 10) / 1024).toFixed(1);

    const diskSize = document.getElementById("vm-disk-size")?.value || "20G";
    const diskFormat = document.getElementById("vm-disk-format")?.value || "qcow2";

    const netEnable = document.getElementById("vm-net-enable")?.checked;
    const sshPort = document.getElementById("vm-ssh-port")?.value.trim();

    document.getElementById("rev-name").textContent = name;
    document.getElementById("rev-id").textContent = id;
    if (document.getElementById("rev-arch")) {
        document.getElementById("rev-arch").textContent = archLabel;
    }
    document.getElementById("rev-os").textContent = os;
    document.getElementById("rev-firmware").textContent = firmware.toUpperCase();
    document.getElementById("rev-iso").textContent = iso;

    document.getElementById("rev-cpu").textContent = `${cpus} Cores`;
    document.getElementById("rev-ram").textContent = `${ram} MB (~${ramGB} GB)`;

    document.getElementById("rev-disk").textContent = diskSize.toUpperCase();
    document.getElementById("rev-disk-format").textContent = diskFormat.toUpperCase();

    if (netEnable) {
        document.getElementById("rev-net").textContent = "Enabled (User-mode NAT)";
        if (sshPort) {
            document.getElementById("rev-ssh").textContent = `:${sshPort} \u2192 guest :22`;
        } else {
            document.getElementById("rev-ssh").textContent = "Enabled (No forwarding)";
        }
    } else {
        document.getElementById("rev-net").textContent = "Disabled";
        document.getElementById("rev-ssh").textContent = "Disabled";
    }
}

// Render real-time HTML5 Canvas telemetry sparklines (CPU, RAM, Storage) for VM summary
function renderVMTelemetryCharts(pushPoint = true) {
    const canvases = document.querySelectorAll(".vm-telemetry-canvas");
    if (!canvases || canvases.length === 0) return;

    window.vmTelemetryHistory = window.vmTelemetryHistory || {};

    const panel = document.getElementById("vm-telemetry");
    const sampleTime = panel ? panel.getAttribute("data-sample-time") : null;
    const lastIngested = panel ? panel.getAttribute("data-last-ingested") : null;

    let shouldPush = pushPoint;
    if (sampleTime) {
        if (sampleTime === lastIngested) {
            shouldPush = false;
        } else if (pushPoint) {
            panel.setAttribute("data-last-ingested", sampleTime);
        }
    }

    const colorConfig = {
        cpu: { stroke: "#58a6ff", fill: "rgba(88, 166, 255, 0.18)" },
        ram: { stroke: "#3fb950", fill: "rgba(63, 185, 80, 0.18)" },
        storage: { stroke: "#d29922", fill: "rgba(210, 153, 34, 0.18)" }
    };

    canvases.forEach((canvas) => {
        const vmId = canvas.getAttribute("data-vmid") || "default";
        const metric = canvas.getAttribute("data-metric") || "cpu";
        const val = parseFloat(canvas.getAttribute("data-value")) || 0;
        const status = canvas.getAttribute("data-status");

        if (!window.vmTelemetryHistory[vmId]) {
            window.vmTelemetryHistory[vmId] = {};
        }
        const vmHist = window.vmTelemetryHistory[vmId];

        const isRunning = status === "running";
        let sampleVal = (isRunning || metric === "storage") ? val : 0;
        if (metric === "cpu" || metric === "ram" || metric === "storage") {
            sampleVal = Math.max(0, Math.min(100, sampleVal));
        }

        if (!vmHist[metric]) {
            vmHist[metric] = [sampleVal];
        } else if (shouldPush) {
            vmHist[metric].push(sampleVal);
            if (vmHist[metric].length > 30) {
                vmHist[metric].shift();
            }
        }

        const data = vmHist[metric];
        const conf = colorConfig[metric] || colorConfig.cpu;

        const wrap = canvas.parentElement;
        const rect = canvas.getBoundingClientRect();
        const wrapW = wrap ? wrap.clientWidth : 0;
        const wrapH = wrap ? wrap.clientHeight : 0;

        // If the canvas container is completely hidden (e.g. inactive tab), skip drawing until visible
        if (wrapW === 0 && (!rect || rect.width === 0)) {
            return;
        }

        const width = Math.floor((rect && rect.width > 0) ? rect.width : (wrapW > 0 ? wrapW : 340));
        const height = Math.floor((rect && rect.height > 0) ? rect.height : (wrapH > 0 ? wrapH : 68));
        const dpr = window.devicePixelRatio || 1;

        const targetW = Math.max(50, width);
        const targetH = Math.max(20, height);

        canvas.width = Math.floor(targetW * dpr);
        canvas.height = Math.floor(targetH * dpr);

        const ctx = canvas.getContext("2d");
        if (!ctx) return;

        ctx.save();
        ctx.setTransform(1, 0, 0, 1, 0, 0);
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        ctx.scale(dpr, dpr);

        const padL = 28;
        const padR = 6;
        const padT = 5;
        const padB = 14;
        const plotW = Math.max(10, targetW - padL - padR);
        const plotH = Math.max(10, targetH - padT - padB);

        // Horizontal grid lines at 0, 50, 100
        ctx.lineWidth = 1;
        ctx.font = "8px ui-monospace, SFMono-Regular, Menlo, monospace";
        ctx.textAlign = "right";
        ctx.textBaseline = "middle";

        [0, 50, 100].forEach((pct) => {
            const y = padT + plotH - (pct / 100) * plotH;
            ctx.strokeStyle = "rgba(110, 118, 129, 0.12)";
            ctx.beginPath();
            ctx.moveTo(padL, y);
            ctx.lineTo(padL + plotW, y);
            ctx.stroke();

            ctx.fillStyle = "#6e7681";
            ctx.fillText(`${pct}%`, padL - 4, y);
        });

        // Time axis indicators
        ctx.fillStyle = "#6e7681";
        ctx.textAlign = "left";
        ctx.fillText("-2.5m", padL, targetH - 3);
        ctx.textAlign = "right";
        ctx.fillText("now", padL + plotW, targetH - 3);

        // Draw sparkline curve
        const maxPoints = 30;
        const stepX = plotW / (maxPoints - 1);

        if (data.length === 1) {
            const v = Math.max(0, Math.min(100, data[0]));
            const y = padT + plotH - (v / 100) * plotH;
            ctx.beginPath();
            ctx.moveTo(padL, y);
            ctx.lineTo(padL + plotW, y);
            ctx.strokeStyle = conf.stroke;
            ctx.lineWidth = 1.5;
            ctx.stroke();

            ctx.beginPath();
            ctx.arc(padL + plotW, y, 2.5, 0, Math.PI * 2);
            ctx.fillStyle = conf.stroke;
            ctx.fill();
        } else {
            const startX = padL + plotW - (data.length - 1) * stepX;
            ctx.beginPath();
            data.forEach((v, i) => {
                const clamped = Math.max(0, Math.min(100, v));
                const x = startX + i * stepX;
                const y = padT + plotH - (clamped / 100) * plotH;
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            });
            ctx.strokeStyle = conf.stroke;
            ctx.lineWidth = 1.75;
            ctx.stroke();

            if (conf.fill) {
                const lastX = padL + plotW;
                const baseY = padT + plotH;
                ctx.beginPath();
                ctx.moveTo(startX, baseY);
                data.forEach((v, i) => {
                    const clamped = Math.max(0, Math.min(100, v));
                    const x = startX + i * stepX;
                    const y = padT + plotH - (clamped / 100) * plotH;
                    ctx.lineTo(x, y);
                });
                ctx.lineTo(lastX, baseY);
                ctx.closePath();

                const grad = ctx.createLinearGradient(0, padT, 0, baseY);
                grad.addColorStop(0, conf.fill);
                grad.addColorStop(1, "rgba(0, 0, 0, 0)");
                ctx.fillStyle = grad;
                ctx.fill();
            }

            const lastVal = Math.max(0, Math.min(100, data[data.length - 1]));
            const lastX = padL + plotW;
            const lastY = padT + plotH - (lastVal / 100) * plotH;
            ctx.beginPath();
            ctx.arc(lastX, lastY, 2.5, 0, Math.PI * 2);
            ctx.fillStyle = conf.stroke;
            ctx.fill();
        }

        if (!isRunning && metric !== "storage") {
            ctx.fillStyle = "rgba(110, 118, 129, 0.55)";
            ctx.font = "bold 9px sans-serif";
            ctx.textAlign = "center";
            ctx.textBaseline = "middle";
            ctx.fillText("OFFLINE", padL + plotW / 2, padT + plotH / 2);
        }

        ctx.restore();
    });
}

/* ==========================================================================
   Storage Actions: Download ISO & Polished Upload Dropzone
   ========================================================================== */

function initStorageActions() {
    initDownloadISOActions();
    initUploadISOActions();
}

function initDownloadISOActions() {
    const form = document.getElementById("download-iso-form");
    if (!form) return;

    const urlInput = document.getElementById("download-iso-url");
    const nameInput = document.getElementById("download-iso-name");
    const submitBtn = document.getElementById("download-submit-btn");
    const errorBanner = document.getElementById("download-error-banner");
    const progressContainer = document.getElementById("download-progress-container");

    // Auto-detect filename from URL when typed or pasted
    if (urlInput && nameInput) {
        urlInput.addEventListener("input", () => {
            const val = urlInput.value.trim();
            if (val) {
                try {
                    const parsed = new URL(val);
                    const lastPart = parsed.pathname.split("/").pop();
                    if (lastPart && lastPart.toLowerCase().endsWith(".iso") && !nameInput.value) {
                        nameInput.value = decodeURIComponent(lastPart);
                    }
                } catch (_) {
                    // Incomplete URL
                }
            }
        });
    }

    // Async download submission with live spinner
    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        const urlVal = (urlInput ? urlInput.value.trim() : "");
        const nameVal = (nameInput ? nameInput.value.trim() : "");

        if (!urlVal) {
            showToast("Please enter a valid ISO download URL", "error");
            return;
        }

        if (errorBanner) errorBanner.style.display = "none";
        if (progressContainer) progressContainer.style.display = "block";
        if (submitBtn) {
            submitBtn.disabled = true;
            submitBtn.innerHTML = `
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="spin">
                    <line x1="12" y1="2" x2="12" y2="6"></line>
                    <line x1="12" y1="18" x2="12" y2="22"></line>
                    <line x1="4.93" y1="4.93" x2="7.76" y2="7.76"></line>
                    <line x1="16.24" y1="16.24" x2="19.07" y2="19.07"></line>
                    <line x1="2" y1="12" x2="6" y2="12"></line>
                    <line x1="18" y1="12" x2="22" y2="12"></line>
                    <line x1="4.93" y1="19.07" x2="7.76" y2="16.24"></line>
                    <line x1="16.24" y1="7.76" x2="19.07" y2="4.93"></line>
                </svg>
                <span>Downloading...</span>
            `;
        }
        if (urlInput) urlInput.disabled = true;
        if (nameInput) nameInput.disabled = true;

        try {
            const resp = await fetch("/storage/download", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                    "Accept": "application/json"
                },
                body: JSON.stringify({ url: urlVal, name: nameVal })
            });

            if (resp.ok) {
                const res = await resp.json().catch(() => ({}));
                showToast(`ISO image '${res.name || nameVal || "image"}' downloaded successfully!`, "success");
                setTimeout(() => {
                    window.location.reload();
                }, 800);
            } else {
                let errText = "Failed to download ISO";
                try {
                    const errObj = await resp.json();
                    if (errObj.error && errObj.error.message) {
                        errText = errObj.error.message;
                    } else if (errObj.error) {
                        errText = errObj.error;
                    }
                } catch (_) {
                    const txt = await resp.text();
                    if (txt) errText = txt;
                }
                showToast(errText, "error");
                if (errorBanner) {
                    errorBanner.textContent = errText;
                    errorBanner.style.display = "flex";
                }
                resetDownloadForm();
            }
        } catch (err) {
            const msg = err.message || "Network error occurred while requesting ISO download";
            showToast(msg, "error");
            if (errorBanner) {
                errorBanner.textContent = msg;
                errorBanner.style.display = "flex";
            }
            resetDownloadForm();
        }

        function resetDownloadForm() {
            if (progressContainer) progressContainer.style.display = "none";
            if (submitBtn) {
                submitBtn.disabled = false;
                submitBtn.innerHTML = `
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                        <polyline points="7 10 12 15 17 10"/>
                        <line x1="12" y1="15" x2="12" y2="3"/>
                    </svg>
                    <span>Download to Storage</span>
                `;
            }
            if (urlInput) urlInput.disabled = false;
            if (nameInput) nameInput.disabled = false;
        }
    });
}

function initUploadISOActions() {
    const form = document.getElementById("upload-iso-form");
    if (!form) return;

    const fileInput = document.getElementById("iso-file-input");
    const dropzone = document.getElementById("iso-upload-dropzone");
    const previewCard = document.getElementById("iso-file-preview");
    const previewName = document.getElementById("iso-preview-name");
    const previewSize = document.getElementById("iso-preview-size");
    const removeBtn = document.getElementById("iso-preview-remove");
    const uploadBtn = document.getElementById("iso-upload-btn");
    const errorBanner = document.getElementById("upload-error-banner");
    const progressContainer = document.getElementById("iso-upload-progress");
    const progressBar = document.getElementById("iso-progress-bar");
    const progressPercent = document.getElementById("iso-progress-percent");
    const progressBytes = document.getElementById("iso-progress-bytes");
    const statusText = document.getElementById("iso-upload-status-text");

    let selectedFile = null;

    if (!fileInput || !dropzone) return;

    // Open file dialog on dropzone click
    dropzone.addEventListener("click", () => {
        fileInput.click();
    });

    // Drag-and-drop visual events
    ["dragenter", "dragover"].forEach(evtName => {
        dropzone.addEventListener(evtName, (e) => {
            e.preventDefault();
            e.stopPropagation();
            dropzone.classList.add("dropzone-active");
        });
    });

    ["dragleave", "dragend"].forEach(evtName => {
        dropzone.addEventListener(evtName, (e) => {
            e.preventDefault();
            e.stopPropagation();
            dropzone.classList.remove("dropzone-active");
        });
    });

    dropzone.addEventListener("drop", (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.remove("dropzone-active");

        if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
            handleFileSelection(e.dataTransfer.files[0]);
        }
    });

    fileInput.addEventListener("change", () => {
        if (fileInput.files && fileInput.files.length > 0) {
            handleFileSelection(fileInput.files[0]);
        }
    });

    function handleFileSelection(file) {
        if (!file.name.toLowerCase().endsWith(".iso")) {
            showToast("Only .iso image files are supported.", "error");
            if (errorBanner) {
                errorBanner.textContent = "Invalid file type. Please select a file ending in .iso";
                errorBanner.style.display = "flex";
            }
            return;
        }

        selectedFile = file;
        if (errorBanner) errorBanner.style.display = "none";
        dropzone.style.display = "none";
        previewCard.style.display = "flex";
        previewName.textContent = file.name;
        previewSize.textContent = formatBytesJS(file.size);
        uploadBtn.disabled = false;
    }

    if (removeBtn) {
        removeBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            selectedFile = null;
            fileInput.value = "";
            previewCard.style.display = "none";
            dropzone.style.display = "flex";
            uploadBtn.disabled = true;
            if (errorBanner) errorBanner.style.display = "none";
            if (progressContainer) progressContainer.style.display = "none";
        });
    }

    // Interactive progress upload
    form.addEventListener("submit", (e) => {
        e.preventDefault();
        if (!selectedFile) {
            showToast("Please select an ISO file to upload", "error");
            return;
        }

        if (errorBanner) errorBanner.style.display = "none";
        if (progressContainer) progressContainer.style.display = "block";
        uploadBtn.disabled = true;
        if (removeBtn) removeBtn.style.display = "none";

        const preventUnload = (evt) => {
            evt.preventDefault();
            evt.returnValue = "An upload is in progress. Are you sure you want to leave?";
            return evt.returnValue;
        };
        window.addEventListener("beforeunload", preventUnload);

        const xhr = new XMLHttpRequest();
        xhr.open("POST", "/storage/upload");
        xhr.setRequestHeader("Accept", "application/json");

        xhr.upload.onprogress = (evt) => {
            if (evt.lengthComputable) {
                const pct = Math.round((evt.loaded / evt.total) * 100);
                if (progressBar) progressBar.style.width = pct + "%";
                if (progressPercent) progressPercent.textContent = pct + "%";
                if (progressBytes) progressBytes.textContent = `${formatBytesJS(evt.loaded)} / ${formatBytesJS(evt.total)}`;
            }
        };

        xhr.onload = () => {
            window.removeEventListener("beforeunload", preventUnload);
            if (xhr.status >= 200 && xhr.status < 400) {
                if (progressBar) progressBar.style.width = "100%";
                if (progressPercent) progressPercent.textContent = "100%";
                if (statusText) statusText.textContent = "Processing and saving to storage pool...";
                showToast(`ISO image '${selectedFile.name}' uploaded successfully!`, "success");
                setTimeout(() => {
                    window.location.reload();
                }, 800);
            } else {
                let errText = "Upload failed";
                try {
                    const parsed = JSON.parse(xhr.responseText);
                    if (parsed.error) errText = parsed.error;
                } catch (_) {
                    if (xhr.responseText) errText = xhr.responseText;
                }
                showToast(errText, "error");
                if (errorBanner) {
                    errorBanner.textContent = errText;
                    errorBanner.style.display = "flex";
                }
                resetUploadUI();
            }
        };

        xhr.onerror = () => {
            window.removeEventListener("beforeunload", preventUnload);
            showToast("Network error during file upload", "error");
            if (errorBanner) {
                errorBanner.textContent = "Network error during upload. Please check your connection.";
                errorBanner.style.display = "flex";
            }
            resetUploadUI();
        };

        const formData = new FormData();
        formData.append("file", selectedFile);
        xhr.send(formData);

        function resetUploadUI() {
            if (progressContainer) progressContainer.style.display = "none";
            uploadBtn.disabled = false;
            if (removeBtn) removeBtn.style.display = "flex";
        }
    });
}

function formatBytesJS(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// ==========================================
// QEMU VM Logs Viewer & Visibility Toggle
// ==========================================
function toggleLogsVisibility(vmId, isVisible) {
    const toggle = document.getElementById('vm-logs-toggle');
    const body = document.getElementById('qemu-log-body');
    const controls = document.getElementById('logs-active-controls');

    if (!body) return;

    if (toggle && toggle.checked !== isVisible) {
        toggle.checked = isVisible;
    }

    if (isVisible) {
        body.style.display = 'block';
        if (controls) controls.style.display = 'inline-flex';
        localStorage.setItem('tinyvm_logs_visible', 'true');

        // Fetch fresh logs and scroll to bottom
        refreshVMLogs(vmId);
    } else {
        body.style.display = 'none';
        if (controls) controls.style.display = 'none';
        localStorage.setItem('tinyvm_logs_visible', 'false');
    }
}

function toggleLogsCard(vmId, event) {
    if (event && event.target && (event.target.closest('label') || event.target.closest('button') || event.target.closest('input'))) {
        return;
    }
    const toggle = document.getElementById('vm-logs-toggle');
    const currentState = toggle ? toggle.checked : false;
    toggleLogsVisibility(vmId, !currentState);
}

function initLogsToggle(vmId) {
    const toggle = document.getElementById('vm-logs-toggle');
    if (!toggle) return;

    const savedPref = localStorage.getItem('tinyvm_logs_visible');
    const hash = window.location.hash.replace('#', '');
    const shouldShow = (hash === 'logs') || (savedPref === 'true');

    toggleLogsVisibility(vmId, shouldShow);

    if (hash === 'logs') {
        setTimeout(() => {
            const card = document.getElementById('qemu-log-card-section');
            if (card) {
                card.scrollIntoView({ behavior: 'smooth' });
            }
        }, 100);
    }
}

function copyVMLogs() {
    const viewer = document.getElementById('qemu-log-viewer');
    const btnText = document.getElementById('copy-logs-text');
    if (!viewer) return;

    const text = viewer.textContent || '';
    if (!text || text.trim() === '' || text === 'No log entries recorded yet.') {
        if (typeof showToast === 'function') {
            showToast('No log content to copy', 'info');
        }
        return;
    }

    const onCopied = () => {
        if (btnText) {
            const original = btnText.textContent;
            btnText.textContent = 'Copied!';
            setTimeout(() => {
                btnText.textContent = original;
            }, 2000);
        }
        if (typeof showToast === 'function') {
            showToast('Logs copied to clipboard', 'success');
        }
    };

    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(onCopied).catch(() => {
            fallbackCopyText(text, onCopied);
        });
    } else {
        fallbackCopyText(text, onCopied);
    }
}

function fallbackCopyText(text, callback) {
    try {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        if (callback) callback();
    } catch (e) {
        console.error('Copy failed:', e);
        if (typeof showToast === 'function') {
            showToast('Failed to copy logs', 'error');
        }
    }
}

function refreshVMLogs(vmId) {
    const btn = document.getElementById('refresh-logs-btn');
    const viewer = document.getElementById('qemu-log-viewer');
    if (!viewer) return;

    if (btn) {
        btn.disabled = true;
        const span = btn.querySelector('span');
        if (span) span.textContent = 'Refreshing...';
    }

    fetch(`/api/v1/vms/${encodeURIComponent(vmId)}/logs`)
        .then(res => {
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            return res.text();
        })
        .then(text => {
            const wasAtBottom = (viewer.scrollHeight - viewer.clientHeight - viewer.scrollTop) <= 40;
            viewer.textContent = text || 'No log entries recorded yet.';
            if (wasAtBottom) {
                viewer.scrollTop = viewer.scrollHeight;
            }
        })
        .catch(err => {
            console.error('Error refreshing VM logs:', err);
        })
        .finally(() => {
            if (btn) {
                btn.disabled = false;
                const span = btn.querySelector('span');
                if (span) span.textContent = 'Refresh';
            }
        });
}
