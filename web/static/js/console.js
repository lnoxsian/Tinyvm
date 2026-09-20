// TinyVM Interactive Browser Console (noVNC Graphical & xterm.js Terminal)
(function() {
    function initConsole() {
        const wrapper = document.getElementById('console-wrapper');
        if (!wrapper) return;

        const vmId = wrapper.dataset.vmId;
        const isRunning = wrapper.dataset.running === 'true';
        if (!isRunning || !vmId) return;

        const statusDot = document.getElementById('status-dot');
        const statusText = document.getElementById('status-text');

        // Tabs
        const btnVncTab = document.getElementById('tab-btn-vnc');
        const btnSshTab = document.getElementById('tab-btn-ssh');

        // Panes
        const paneVnc = document.getElementById('pane-vnc');
        const paneSsh = document.getElementById('pane-ssh');
        const novncFrame = document.getElementById('novnc-frame');

        // Toolbar controls
        const sshControls = document.getElementById('ssh-controls');
        const sshUserInput = document.getElementById('ssh-user-input');
        const sshModeSelect = document.getElementById('ssh-mode-select');
        const btnSshConnect = document.getElementById('btn-ssh-connect');
        const btnNovncStandalone = document.getElementById('btn-novnc-standalone');
        const btnCAD = document.getElementById('btn-cad');
        const btnReconnect = document.getElementById('btn-reconnect');
        const btnFullscreen = document.getElementById('btn-fullscreen');
        const iconFsEnter = document.getElementById('icon-fullscreen-enter');
        const iconFsExit = document.getElementById('icon-fullscreen-exit');
        const labelFs = document.getElementById('label-fullscreen');

        let currentMode = 'vnc'; // 'vnc' | 'ssh'

        // SSH / Shell terminal session objects
        let sshTerm = null;
        let sshFit = null;
        let sshWS = null;
        let lastCols = 0;
        let lastRows = 0;

        function sendResize(cols, rows) {
            if (!cols || !rows) return;
            if (cols === lastCols && rows === lastRows) return;
            if (sshWS && sshWS.readyState === WebSocket.OPEN) {
                lastCols = cols;
                lastRows = rows;
                sshWS.send(JSON.stringify({ type: 'resize', cols: cols, rows: rows }));
            }
        }

        function setStatus(state, msg) {
            if (!statusDot || !statusText) return;
            statusDot.className = 'console-status-dot ' + state;
            statusText.textContent = msg;
        }

        function getWsUrl(path) {
            const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            return `${proto}//${window.location.host}/api/v1/vms/${encodeURIComponent(vmId)}/${path}`;
        }

        // --- 1. noVNC Graphical Frame ---
        if (novncFrame) {
            novncFrame.addEventListener('load', () => {
                if (currentMode === 'vnc') {
                    setStatus('connected', 'Connected (VNC)');
                }
            });
        }

        // --- 2. xterm.js Terminal (SSH / Host Shell) ---
        function initSSHTerminal() {
            if (sshTerm) return;
            if (!window.Terminal) {
                console.error('xterm.js not loaded');
                return;
            }

            sshTerm = new window.Terminal({
                cursorBlink: true,
                fontSize: 13,
                fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                theme: {
                    background: '#090d16',
                    foreground: '#e6edf3',
                    cursor: '#58a6ff',
                    selectionBackground: '#264f78'
                }
            });

            if (window.FitAddon && window.FitAddon.FitAddon) {
                sshFit = new window.FitAddon.FitAddon();
                sshTerm.loadAddon(sshFit);
            }

            const container = document.getElementById('terminal-ssh-container');
            if (container) {
                sshTerm.open(container);
            }

            sshTerm.onData(data => {
                if (sshWS && sshWS.readyState === WebSocket.OPEN) {
                    sshWS.send(data);
                }
            });

            sshTerm.onResize(size => {
                sendResize(size.cols, size.rows);
            });
        }

        function connectSSH() {
            initSSHTerminal();

            lastCols = 0;
            lastRows = 0;

            if (sshWS) {
                try { sshWS.close(); } catch (e) {}
                sshWS = null;
            }

            const user = (sshUserInput ? sshUserInput.value.trim() : 'root') || 'root';
            const mode = (sshModeSelect ? sshModeSelect.value : 'ssh') || 'ssh';
            const label = mode === 'shell' ? 'Host Shell' : `SSH (${user})`;

            setStatus('connecting', `Connecting ${label}...`);
            const url = `${getWsUrl('ssh')}?user=${encodeURIComponent(user)}&mode=${encodeURIComponent(mode)}`;
            sshWS = new WebSocket(url);

            sshWS.onopen = () => {
                if (currentMode === 'ssh') {
                    setStatus('connected', `Connected (${label})`);
                    if (sshFit) {
                        sshFit.fit();
                    }
                    if (sshTerm) {
                        sendResize(sshTerm.cols, sshTerm.rows);
                        sshTerm.focus();
                    }
                }
            };

            sshWS.onmessage = (event) => {
                if (sshTerm) {
                    sshTerm.write(event.data);
                }
            };

            sshWS.onclose = () => {
                if (currentMode === 'ssh') {
                    setStatus('disconnected', `${label} Disconnected`);
                }
            };

            sshWS.onerror = (err) => {
                console.error('Terminal WebSocket error:', err);
                if (currentMode === 'ssh') {
                    setStatus('disconnected', `${label} Error`);
                }
            };
        }

        // Reconnect when username or mode select changes
        if (sshModeSelect) {
            sshModeSelect.addEventListener('change', () => {
                if (currentMode === 'ssh') {
                    connectSSH();
                }
            });
        }
        if (sshUserInput) {
            sshUserInput.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' && currentMode === 'ssh') {
                    connectSSH();
                }
            });
        }
        if (btnSshConnect) {
            btnSshConnect.addEventListener('click', () => {
                connectSSH();
            });
        }

        // --- Tab Switching ---
        function switchMode(mode) {
            currentMode = mode;

            if (btnVncTab) btnVncTab.classList.toggle('active', mode === 'vnc');
            if (btnSshTab) btnSshTab.classList.toggle('active', mode === 'ssh');

            if (paneVnc) paneVnc.classList.toggle('active', mode === 'vnc');
            if (paneSsh) paneSsh.classList.toggle('active', mode === 'ssh');

            if (btnCAD) btnCAD.style.display = (mode === 'vnc') ? 'inline-flex' : 'none';
            if (btnNovncStandalone) btnNovncStandalone.style.display = (mode === 'vnc') ? 'inline-flex' : 'none';
            if (sshControls) sshControls.style.display = (mode === 'ssh') ? 'inline-flex' : 'none';

            if (mode === 'vnc') {
                setStatus('connected', 'Connected (VNC)');
            } else if (mode === 'ssh') {
                if (!sshWS || sshWS.readyState !== WebSocket.OPEN) {
                    connectSSH();
                } else {
                    const user = (sshUserInput ? sshUserInput.value.trim() : 'root') || 'root';
                    const sMode = (sshModeSelect ? sshModeSelect.value : 'ssh') || 'ssh';
                    const label = sMode === 'shell' ? 'Host Shell' : `SSH (${user})`;
                    setStatus('connected', `Connected (${label})`);
                    if (sshFit) sshFit.fit();
                    if (sshTerm) sshTerm.focus();
                }
            }
        }

        if (btnVncTab) btnVncTab.addEventListener('click', () => switchMode('vnc'));
        if (btnSshTab) btnSshTab.addEventListener('click', () => switchMode('ssh'));

        // --- CAD (Ctrl+Alt+Del) ---
        if (btnCAD) {
            btnCAD.addEventListener('click', () => {
                if (novncFrame && novncFrame.contentWindow) {
                    try {
                        const win = novncFrame.contentWindow;
                        if (win.UI && win.UI.rfb && typeof win.UI.rfb.sendCtrlAltDel === 'function') {
                            win.UI.rfb.sendCtrlAltDel();
                            return;
                        }
                    } catch (e) {
                        console.warn('Unable to invoke sendCtrlAltDel via iframe contentWindow:', e);
                    }
                }
            });
        }

        // --- Reconnect ---
        if (btnReconnect) {
            btnReconnect.addEventListener('click', () => {
                if (currentMode === 'vnc') {
                    if (novncFrame) {
                        setStatus('connecting', 'Reconnecting VNC...');
                        novncFrame.src = novncFrame.src;
                    }
                } else if (currentMode === 'ssh') {
                    connectSSH();
                }
            });
        }

        // --- Fullscreen Toggle ---
        if (btnFullscreen) {
            btnFullscreen.addEventListener('click', () => {
                if (!document.fullscreenElement) {
                    wrapper.requestFullscreen().catch(err => {
                        console.error('Fullscreen request error:', err);
                    });
                } else {
                    document.exitFullscreen().catch(err => {
                        console.error('Exit fullscreen error:', err);
                    });
                }
            });
        }

        document.addEventListener('fullscreenchange', () => {
            const isFs = !!document.fullscreenElement;
            if (iconFsEnter) iconFsEnter.style.display = isFs ? 'none' : 'inline-block';
            if (iconFsExit) iconFsExit.style.display = isFs ? 'inline-block' : 'none';
            if (labelFs) labelFs.textContent = isFs ? 'Exit' : 'Fullscreen';

            if (currentMode === 'ssh' && sshFit) {
                setTimeout(() => sshFit.fit(), 100);
            }
        });

        window.addEventListener('resize', () => {
            if (currentMode === 'ssh' && sshFit) {
                sshFit.fit();
            }
        });

        // Set default view to graphical VNC
        switchMode('vnc');
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initConsole);
    } else {
        initConsole();
    }
})();
