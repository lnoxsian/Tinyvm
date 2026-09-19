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
});

// Floating toast notification system
function showToast(message, type = "success") {
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
    } else if (type === "error") {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`;
    } else {
        iconSvg = `<svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>`;
    }

    toast.innerHTML = `${iconSvg}<span>${message}</span>`;
    container.appendChild(toast);

    requestAnimationFrame(() => toast.classList.add("show"));

    setTimeout(() => {
        toast.classList.remove("show");
        setTimeout(() => toast.remove(), 250);
    }, 3500);
}

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
        alert(msg);
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

    // Highlight active preset button if matches
    const presets = document.querySelectorAll("#wizard-panel-2 .btn-preset");
    presets.forEach((btn) => {
        btn.classList.toggle("active", btn.textContent.includes(`${val} MB`));
    });
}

function setRAMPreset(mb, btn) {
    syncRAM(mb);
    const presets = document.querySelectorAll("#wizard-panel-2 .btn-preset");
    presets.forEach((b) => b.classList.remove("active"));
    if (btn) btn.classList.add("active");
}

function setDiskPreset(sizeStr) {
    const diskInput = document.getElementById("vm-disk-size");
    if (diskInput) {
        diskInput.value = sizeStr;
    }
    const presets = document.querySelectorAll("#wizard-panel-3 .btn-preset");
    presets.forEach((btn) => {
        btn.classList.toggle("active", btn.textContent.includes(sizeStr.replace("G", " GB")));
    });
}

function toggleNetworkFields(checked) {
    const fields = document.getElementById("network-fields");
    if (fields) {
        fields.style.display = checked ? "block" : "none";
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
