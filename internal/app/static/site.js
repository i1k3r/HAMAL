(() => {
  // --------------------------------------------------------------------------
  // Theme Management (Light / Dark Mode with LocalStorage Persistence)
  // --------------------------------------------------------------------------
  function initTheme() {
    const themeBtn = document.getElementById('theme-toggle-btn');
    if (!themeBtn) return;

    themeBtn.addEventListener('click', () => {
      const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
      const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', newTheme);
      localStorage.setItem('hamal_theme', newTheme);
    });
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initTheme);
  } else {
    initTheme();
  }

  // --------------------------------------------------------------------------
  // 1. Room Creation Form Handler (Home Page)
  // --------------------------------------------------------------------------
  const createForm = document.getElementById('create-room-form');
  if (createForm) {
    createForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const createBtn = document.getElementById('create-btn');
      const formError = document.getElementById('form-error');
      const ttlSelect = document.getElementById('ttl-select');
      const pinInput = document.getElementById('pin-input');
      const ttlSeconds = parseInt(ttlSelect ? ttlSelect.value : '3600', 10);
      const pin = pinInput ? pinInput.value.trim() : '';

      if (createBtn) {
        createBtn.disabled = true;
        createBtn.textContent = 'Creating room…';
      }
      if (formError) formError.style.display = 'none';

      try {
        const res = await fetch('/api/v1/rooms', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ttl_seconds: ttlSeconds, pin: pin }),
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({ error: 'Failed to create room' }));
          throw new Error(errData.error || 'Server rejected room creation');
        }

        const data = await res.json();
        if (data.creator_url) {
          window.location.href = data.creator_url;
        } else {
          throw new Error('Missing creator URL in response');
        }
      } catch (err) {
        if (formError) {
          formError.textContent = err.message;
          formError.style.display = 'block';
        }
        if (createBtn) {
          createBtn.disabled = false;
          createBtn.textContent = 'Create Room';
        }
      }
    });
  }

  // --------------------------------------------------------------------------
  // Brand Story Accordion Drawer (Home Page)
  // --------------------------------------------------------------------------
  const storyToggle = document.getElementById('brand-story-toggle');
  if (storyToggle) {
    storyToggle.addEventListener('click', () => {
      const isExpanded = storyToggle.getAttribute('aria-expanded') === 'true';
      storyToggle.setAttribute('aria-expanded', String(!isExpanded));
    });
  }

  // --------------------------------------------------------------------------
  // 2. Room Management (Creator & Participant Views)
  // --------------------------------------------------------------------------
  const page = document.body.dataset.page;
  if (page === 'creator' || page === 'participant') {
    const token = document.body.dataset.token;
    const expiresAtStr = document.body.dataset.expires;
    const expiresAt = expiresAtStr ? new Date(expiresAtStr).getTime() : 0;
    const globalShareEnabled = document.body.dataset.globalShareEnabled === 'true';

    const countdownEl = document.getElementById('countdown');
    const statusBadge = document.getElementById('status-badge');
    const activeCard = document.getElementById('room-active-card');
    const inactiveCard = document.getElementById('room-inactive-card');
    const pinCard = document.getElementById('room-pin-card');
    const inactiveTitle = document.getElementById('inactive-title');
    const inactiveMsg = document.getElementById('inactive-message');
    const lockoutAlert = document.getElementById('lockout-alert');
    const unlockRoomBtn = document.getElementById('unlock-room-btn');

    let isTerminated = false;
    let pollTimer = null;

    function formatTime(totalSeconds) {
      if (totalSeconds <= 0) return '00:00';
      const hours = Math.floor(totalSeconds / 3600);
      const minutes = Math.floor((totalSeconds % 3600) / 60);
      const seconds = totalSeconds % 60;

      const pad = (n) => String(n).padStart(2, '0');
      if (hours > 0) {
        return `${hours}h ${pad(minutes)}m ${pad(seconds)}s`;
      }
      return `${pad(minutes)}:${pad(seconds)}`;
    }

    function formatBytes(bytes) {
      if (bytes === 0) return '0 Bytes';
      const k = 1024;
      const sizes = ['Bytes', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    function getFileIconSVG(filename, contentType) {
      const ext = (filename.split('.').pop() || '').toLowerCase();
      const type = (contentType || '').toLowerCase();

      // Images
      if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico'].includes(ext) || type.startsWith('image/')) {
        return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
          <circle cx="8.5" cy="8.5" r="1.5"></circle>
          <polyline points="21 15 16 10 5 21"></polyline>
        </svg>`;
      }

      // Videos
      if (['mp4', 'mkv', 'avi', 'mov', 'webm', 'wmv'].includes(ext) || type.startsWith('video/')) {
        return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <polygon points="23 7 16 12 23 17 23 7"></polygon>
          <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
        </svg>`;
      }

      // Audio
      if (['mp3', 'wav', 'flac', 'ogg', 'm4a', 'aac'].includes(ext) || type.startsWith('audio/')) {
        return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 18V5l12-2v13"></path>
          <circle cx="6" cy="18" r="3"></circle>
          <circle cx="18" cy="16" r="3"></circle>
        </svg>`;
      }

      // Archives
      if (['zip', 'rar', 'tar', 'gz', '7z', 'bz2', 'xz'].includes(ext)) {
        return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 8v13H3V8"></path>
          <path d="M1 3h22v5H1z"></path>
          <path d="M10 12h4"></path>
        </svg>`;
      }

      // Code / Text
      if (['js', 'ts', 'html', 'css', 'json', 'py', 'go', 'rs', 'c', 'cpp', 'sh', 'md', 'xml', 'yaml', 'yml'].includes(ext)) {
        return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="16 18 22 12 16 6"></polyline>
          <polyline points="8 6 2 12 8 18"></polyline>
        </svg>`;
      }

      // Documents (PDF, Doc, etc.) / Default
      return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
        <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"></path>
        <polyline points="14 2 14 8 20 8"></polyline>
      </svg>`;
    }

    function showInactive(title, message) {
      isTerminated = true;
      if (pollTimer) clearTimeout(pollTimer);
      if (activeCard) activeCard.style.display = 'none';
      if (pinCard) pinCard.style.display = 'none';
      if (inactiveCard) {
        if (inactiveTitle) inactiveTitle.textContent = title;
        if (inactiveMsg) inactiveMsg.textContent = message;
        inactiveCard.style.display = 'block';
      }
    }

    // Countdown loop with visual warning when under 10 minutes
    function updateCountdown() {
      if (isTerminated) return;
      const now = Date.now();
      const remainingMs = expiresAt - now;
      const remainingSec = Math.max(0, Math.floor(remainingMs / 1000));

      if (countdownEl) {
        countdownEl.textContent = formatTime(remainingSec);
        if (remainingSec > 0 && remainingSec < 600) {
          countdownEl.classList.add('timer-warning');
          if (statusBadge) {
            statusBadge.className = 'badge badge-warning';
            statusBadge.innerHTML = '<span class="badge-dot"></span>EXPIRING SOON';
          }
        } else {
          countdownEl.classList.remove('timer-warning');
          if (statusBadge) {
            statusBadge.className = 'badge badge-active';
            statusBadge.innerHTML = '<span class="badge-dot"></span>ROOM ACTIVE';
          }
        }
      }

      if (remainingSec <= 0 && expiresAt > 0) {
        showInactive('Room Expired', 'This temporary room has reached its lifespan and is no longer accessible.');
        return;
      }
      setTimeout(updateCountdown, 1000);
    }
    updateCountdown();

    // Polling function for room status and files
    async function pollStatus() {
      if (isTerminated || !token) return;

      try {
        // Poll room status
        const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}`, {
          cache: 'no-store',
        });

        if (res.status === 404 || res.status === 410) {
          showInactive('Room Inactive', 'This temporary room is no longer accessible.');
          return;
        }

        if (res.ok) {
          const data = await res.json();
          if (data.status === 'closed') {
            showInactive('Room Closed', 'This room was closed by the creator.');
            return;
          } else if (data.status === 'expired' || data.remaining_seconds <= 0) {
            showInactive('Room Expired', 'This temporary room has expired.');
            return;
          }

          if (page === 'creator' && lockoutAlert) {
            lockoutAlert.style.display = data.is_locked ? 'flex' : 'none';
          }

          if (page === 'participant') {
            const pinCooldown = document.getElementById('pin-cooldown');
            const pinCooldownText = document.getElementById('pin-cooldown-text');
            const unlockBtn = document.getElementById('unlock-btn');

            if (pinCooldown) {
              if (data.is_locked) {
                pinCooldown.style.display = 'block';
                if (pinCooldownText) {
                  pinCooldownText.textContent = `Too many failed attempts. Cooldown active (${formatTime(data.retry_after_seconds)} remaining).`;
                }
                if (unlockBtn) unlockBtn.disabled = true;
              } else {
                pinCooldown.style.display = 'none';
                if (unlockBtn) unlockBtn.disabled = false;
              }
            }

            if (data.pin_required && !data.pin_authenticated) {
              if (pinCard) pinCard.style.display = 'block';
              if (activeCard) activeCard.style.display = 'none';
            } else {
              if (pinCard) pinCard.style.display = 'none';
              if (activeCard) activeCard.style.display = 'block';
            }
          }
        }

        // Poll file list if not blocked by PIN
        const filesRes = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/files`, {
          cache: 'no-store',
        });
        if (filesRes.ok) {
          const filesData = await filesRes.json();
          renderFileList(filesData.files || []);
        }
      } catch (e) {
        // Network glitches are ignored during polling
      }

      const nextInterval = document.hidden ? 15000 : 4000;
      pollTimer = setTimeout(pollStatus, nextInterval);
    }

    function renderFileList(files) {
      const fileListEl = document.getElementById('file-list');
      const fileCountEl = document.getElementById('file-count');
      if (!fileListEl) return;

      if (fileCountEl) fileCountEl.textContent = files.length;

      if (files.length === 0) {
        fileListEl.innerHTML = `
          <div id="no-files-msg" class="empty-state">
            <svg class="empty-state-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"></path>
              <polyline points="3.27 6.96 12 12.01 20.73 6.96"></polyline>
              <line x1="12" y1="22.08" x2="12" y2="12"></line>
            </svg>
            <p class="empty-state-title">No files uploaded yet</p>
            <p class="empty-state-text">Drop parcels above or browse from your device. All connected participants will receive files in real time.</p>
          </div>
        `;
        return;
      }

      fileListEl.innerHTML = '';
      const orderedFiles = [...files].reverse();
      orderedFiles.forEach((file) => {
        const item = document.createElement('div');
        item.className = 'file-item';
        item.dataset.fileId = file.file_id;

        const mainDiv = document.createElement('div');
        mainDiv.className = 'file-item-main';

        const iconDiv = document.createElement('div');
        iconDiv.className = 'file-type-icon';
        iconDiv.innerHTML = getFileIconSVG(file.filename, file.content_type);

        const info = document.createElement('div');
        info.className = 'file-info';

        const nameSpan = document.createElement('span');
        nameSpan.className = 'file-name';
        nameSpan.title = file.filename;
        nameSpan.textContent = file.filename; // XSS-safe

        const metaSpan = document.createElement('span');
        metaSpan.className = 'file-meta font-mono';
        metaSpan.textContent = `${formatBytes(file.size_bytes)} · ${file.content_type}`;

        info.appendChild(nameSpan);
        info.appendChild(metaSpan);

        mainDiv.appendChild(iconDiv);
        mainDiv.appendChild(info);

        const actions = document.createElement('div');
        actions.className = 'file-actions';

        if (globalShareEnabled && page === 'creator') {
          const shareBtn = document.createElement('button');
          shareBtn.type = 'button';
          shareBtn.className = 'btn btn-secondary btn-sm btn-share-link';
          shareBtn.dataset.fileId = file.file_id;
          shareBtn.dataset.fileName = file.filename;
          shareBtn.textContent = 'Share Link';
          actions.appendChild(shareBtn);
        }

        const downloadLink = document.createElement('a');
        downloadLink.className = 'btn btn-secondary btn-sm btn-download';
        downloadLink.href = `/api/v1/rooms/${encodeURIComponent(token)}/files/${encodeURIComponent(file.file_id)}`;
        downloadLink.download = file.filename;
        downloadLink.innerHTML = `
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
            <polyline points="7 10 12 15 17 10"></polyline>
            <line x1="12" y1="15" x2="12" y2="3"></line>
          </svg>
          Download
        `;

        actions.appendChild(downloadLink);

        item.appendChild(mainDiv);
        item.appendChild(actions);
        fileListEl.appendChild(item);
      });
    }

    // Start polling with initial delay
    pollTimer = setTimeout(pollStatus, 4000);

    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && !isTerminated) {
        if (pollTimer) clearTimeout(pollTimer);
        pollStatus();
      }
    });

    // --------------------------------------------------------------------------
    // 3. Participant PIN Authentication Handler
    // --------------------------------------------------------------------------
    const pinForm = document.getElementById('pin-form');
    if (pinForm) {
      pinForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const pinInput = document.getElementById('participant-pin-input');
        const unlockBtn = document.getElementById('unlock-btn');
        const pinError = document.getElementById('pin-error');
        const pinCooldown = document.getElementById('pin-cooldown');
        const pinCooldownText = document.getElementById('pin-cooldown-text');

        const pinVal = pinInput ? pinInput.value.trim() : '';
        if (!pinVal) return;

        if (unlockBtn) {
          unlockBtn.disabled = true;
          unlockBtn.textContent = 'Verifying PIN…';
        }
        if (pinError) pinError.style.display = 'none';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/auth/pin`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ pin: pinVal }),
          });

          if (res.status === 200) {
            if (pinCard) pinCard.style.display = 'none';
            if (activeCard) activeCard.style.display = 'block';
            pollStatus();
          } else if (res.status === 401) {
            const errData = await res.json().catch(() => ({}));
            let msg = 'Incorrect PIN';
            if (errData.remaining_attempts !== undefined) {
              msg += ` (${errData.remaining_attempts} attempts remaining)`;
            }
            if (pinError) {
              pinError.textContent = msg;
              pinError.style.display = 'block';
            }
          } else if (res.status === 429) {
            const errData = await res.json().catch(() => ({}));
            const retrySec = errData.retry_after_seconds || 300;
            if (pinCooldown) {
              pinCooldown.style.display = 'block';
              if (pinCooldownText) {
                pinCooldownText.textContent = `Too many failed attempts. Cooldown active (${formatTime(retrySec)} remaining).`;
              }
            }
          } else if (res.status === 404 || res.status === 410) {
            showInactive('Room Inactive', 'This temporary room is no longer accessible.');
          } else {
            const errData = await res.json().catch(() => ({}));
            if (pinError) {
              pinError.textContent = errData.error || 'Authentication error';
              pinError.style.display = 'block';
            }
          }
        } catch (err) {
          if (pinError) {
            pinError.textContent = 'Network error while verifying PIN';
            pinError.style.display = 'block';
          }
        } finally {
          if (unlockBtn) {
            unlockBtn.disabled = false;
            unlockBtn.textContent = 'Unlock Room';
          }
          if (pinInput) pinInput.value = '';
        }
      });
    }

    // --------------------------------------------------------------------------
    // 4. File Upload Handling (Drag & Drop + Streaming Progress)
    // --------------------------------------------------------------------------
    const dropzone = document.getElementById('dropzone');
    const dropzoneTitle = document.getElementById('dropzone-title');
    const fileInput = document.getElementById('file-input');
    const progressContainer = document.getElementById('upload-progress-container');
    const progressFilename = document.getElementById('upload-filename');
    const progressPercent = document.getElementById('upload-percent');
    const progressFill = document.getElementById('progress-bar-fill');
    const uploadError = document.getElementById('upload-error');

    let isUploading = false;
    const uploadQueue = [];

    function handleFiles(files) {
      if (isTerminated || !files || files.length === 0) return;
      for (let i = 0; i < files.length; i++) {
        uploadQueue.push(files[i]);
      }
      if (!isUploading) {
        processNextUpload();
      }
    }

    function processNextUpload() {
      if (uploadQueue.length === 0) {
        isUploading = false;
        if (!uploadError || uploadError.style.display === 'none') {
          setTimeout(() => {
            if (progressContainer && (!uploadError || uploadError.style.display === 'none')) {
              progressContainer.style.display = 'none';
            }
          }, 1500);
        }
        return;
      }

      isUploading = true;
      const file = uploadQueue.shift();

      if (progressContainer) progressContainer.style.display = 'block';
      if (uploadError) uploadError.style.display = 'none';
      if (progressFilename) progressFilename.textContent = file.name;
      if (progressPercent) progressPercent.textContent = '0%';
      if (progressFill) {
        progressFill.style.width = '0%';
        progressFill.style.backgroundColor = 'var(--accent-amber)';
      }

      // Pre-check maximum file size (10 GiB = 10,737,418,240 bytes)
      const maxUploadBytes = 10 * 1024 * 1024 * 1024;
      if (file.size && file.size > maxUploadBytes) {
        showUploadError(`${file.name}: File exceeds the maximum upload size of 10 GiB.`);
        processNextUpload();
        return;
      }

      const formData = new FormData();
      formData.append('file', file);

      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/v1/rooms/${encodeURIComponent(token)}/files`, true);

      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          const percent = Math.round((e.loaded / e.total) * 100);
          if (progressPercent) progressPercent.textContent = `${percent}%`;
          if (progressFill) progressFill.style.width = `${percent}%`;
        }
      };

      xhr.onload = () => {
        if (xhr.status === 201) {
          if (progressPercent) progressPercent.textContent = '100%';
          if (progressFill) progressFill.style.width = '100%';
          pollStatus();
          setTimeout(processNextUpload, 300);
        } else {
          let errMsg = 'Upload failed';
          if (xhr.status === 413) {
            errMsg = 'File exceeds the maximum upload size of 10 GiB or room quota.';
          } else {
            try {
              const data = JSON.parse(xhr.responseText);
              if (data.error) errMsg = data.error;
            } catch (_) {}
          }
          showUploadError(`${file.name}: ${errMsg}`);
          processNextUpload();
        }
      };

      xhr.onerror = () => {
        if (file.size && file.size > maxUploadBytes) {
          showUploadError(`${file.name}: File exceeds the maximum upload size of 10 GiB.`);
        } else {
          showUploadError(`${file.name}: Upload failed or connection interrupted.`);
        }
        processNextUpload();
      };

      xhr.send(formData);
    }

    function showUploadError(msg) {
      if (uploadError) {
        uploadError.textContent = msg;
        uploadError.style.display = 'block';
      }
      if (progressContainer) {
        progressContainer.style.display = 'block';
      }
      if (progressFill) {
        progressFill.style.backgroundColor = 'var(--accent-danger)';
      }
    }

    if (dropzone && fileInput) {
      dropzone.addEventListener('click', (e) => {
        if (e.target !== fileInput) {
          fileInput.click();
        }
      });

      dropzone.addEventListener('dragover', (e) => {
        e.preventDefault();
        dropzone.classList.add('dragover');
        if (dropzoneTitle) dropzoneTitle.textContent = 'Drop to upload';
      });

      dropzone.addEventListener('dragleave', () => {
        dropzone.classList.remove('dragover');
        if (dropzoneTitle) dropzoneTitle.textContent = 'Drag & drop files here';
      });

      dropzone.addEventListener('drop', (e) => {
        e.preventDefault();
        dropzone.classList.remove('dragover');
        if (dropzoneTitle) dropzoneTitle.textContent = 'Drag & drop files here';
        if (e.dataTransfer && e.dataTransfer.files) {
          handleFiles(e.dataTransfer.files);
        }
      });

      fileInput.addEventListener('change', (e) => {
        if (e.target.files) {
          handleFiles(e.target.files);
          fileInput.value = '';
        }
      });
    }

    // --------------------------------------------------------------------------
    // Interactive QR Code Expand / Return (Creator Page)
    // --------------------------------------------------------------------------
    const qrBox = document.getElementById('qr-box');
    const qrImage = document.getElementById('qr-image');
    let isQRExpanded = false;
    let qrExpandedCard = null;
    let qrBackdrop = null;

    function expandQR() {
      if (isQRExpanded || !qrBox || !qrImage) return;
      isQRExpanded = true;
      qrBox.setAttribute('aria-expanded', 'true');

      const startRect = qrBox.getBoundingClientRect();

      // Create or reuse backdrop overlay
      if (!qrBackdrop) {
        qrBackdrop = document.createElement('div');
        qrBackdrop.className = 'qr-lightbox-backdrop';
        document.body.appendChild(qrBackdrop);
        qrBackdrop.addEventListener('click', collapseQR);
      }
      qrBackdrop.classList.add('active');

      // Create expanded floating card starting at exact startRect
      qrExpandedCard = document.createElement('div');
      qrExpandedCard.className = 'qr-expanded-card';
      qrExpandedCard.tabIndex = 0;
      qrExpandedCard.setAttribute('role', 'button');
      qrExpandedCard.setAttribute('aria-label', 'Return QR code to normal size');

      const cloneImg = document.createElement('img');
      cloneImg.src = qrImage.src;
      cloneImg.alt = qrImage.alt;
      qrExpandedCard.appendChild(cloneImg);

      qrExpandedCard.style.top = `${startRect.top}px`;
      qrExpandedCard.style.left = `${startRect.left}px`;
      qrExpandedCard.style.width = `${startRect.width}px`;
      qrExpandedCard.style.height = `${startRect.height}px`;
      qrExpandedCard.style.padding = '1.125rem';

      document.body.appendChild(qrExpandedCard);

      // Hide the in-flow original to prevent duplication
      qrBox.style.visibility = 'hidden';
      document.body.style.overflow = 'hidden';

      // Force layout reflow
      qrExpandedCard.offsetHeight;

      // Calculate target centered bounding box
      const maxWidth = Math.min(window.innerWidth * 0.88, 440);
      const maxHeight = Math.min(window.innerHeight * 0.88, 440);
      const targetSize = Math.max(280, Math.min(maxWidth, maxHeight));
      const targetLeft = Math.round((window.innerWidth - targetSize) / 2);
      const targetTop = Math.round((window.innerHeight - targetSize) / 2);

      // Animate smoothly to center
      qrExpandedCard.style.top = `${targetTop}px`;
      qrExpandedCard.style.left = `${targetLeft}px`;
      qrExpandedCard.style.width = `${targetSize}px`;
      qrExpandedCard.style.height = `${targetSize}px`;
      qrExpandedCard.style.padding = '1.75rem';

      // Event handlers for closing
      qrExpandedCard.addEventListener('click', collapseQR);
      qrExpandedCard.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Escape') {
          e.preventDefault();
          collapseQR();
        }
      });
      setTimeout(() => {
        if (qrExpandedCard) qrExpandedCard.focus();
      }, 50);
    }

    function collapseQR() {
      if (!isQRExpanded || !qrExpandedCard) return;
      isQRExpanded = false;
      qrBox.setAttribute('aria-expanded', 'false');

      if (qrBackdrop) {
        qrBackdrop.classList.remove('active');
      }

      const returnRect = qrBox.getBoundingClientRect();

      qrExpandedCard.style.top = `${returnRect.top}px`;
      qrExpandedCard.style.left = `${returnRect.left}px`;
      qrExpandedCard.style.width = `${returnRect.width}px`;
      qrExpandedCard.style.height = `${returnRect.height}px`;
      qrExpandedCard.style.padding = '1.125rem';
      qrExpandedCard.style.borderRadius = 'var(--radius-lg)';

      const prefersReduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      const animDuration = prefersReduced ? 0 : 300;

      setTimeout(() => {
        if (qrExpandedCard) {
          qrExpandedCard.remove();
          qrExpandedCard = null;
        }
        if (qrBox) {
          qrBox.style.visibility = '';
          qrBox.focus();
        }
        document.body.style.overflow = '';
      }, animDuration);
    }

    if (qrBox) {
      qrBox.addEventListener('click', () => {
        if (!isQRExpanded) expandQR();
        else collapseQR();
      });

      qrBox.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          if (!isQRExpanded) expandQR();
          else collapseQR();
        }
      });
    }

    // Global Escape key listener to return QR
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && isQRExpanded) {
        collapseQR();
      }
    });

    // Window resize handler while expanded to keep centered
    window.addEventListener('resize', () => {
      if (isQRExpanded && qrExpandedCard) {
        const maxWidth = Math.min(window.innerWidth * 0.88, 440);
        const maxHeight = Math.min(window.innerHeight * 0.88, 440);
        const targetSize = Math.max(280, Math.min(maxWidth, maxHeight));
        const targetLeft = Math.round((window.innerWidth - targetSize) / 2);
        const targetTop = Math.round((window.innerHeight - targetSize) / 2);
        qrExpandedCard.style.top = `${targetTop}px`;
        qrExpandedCard.style.left = `${targetLeft}px`;
        qrExpandedCard.style.width = `${targetSize}px`;
        qrExpandedCard.style.height = `${targetSize}px`;
      }
    });

    // --------------------------------------------------------------------------
    // 5. Copy Link Handler (Creator Page)
    // --------------------------------------------------------------------------
    const copyBtn = document.getElementById('copy-link-btn');
    const linkInput = document.getElementById('participant-link-input');
    const copyToast = document.getElementById('copy-toast');

    if (copyBtn && linkInput) {
      copyBtn.addEventListener('click', async () => {
        try {
          if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(linkInput.value);
          } else {
            linkInput.select();
            document.execCommand('copy');
          }
          if (copyToast) {
            copyToast.classList.add('show');
            setTimeout(() => copyToast.classList.remove('show'), 2500);
          }
          const origText = copyBtn.textContent;
          copyBtn.textContent = 'Copied!';
          setTimeout(() => { copyBtn.textContent = origText; }, 2000);
        } catch (err) {
          linkInput.select();
        }
      });
    }

    // --------------------------------------------------------------------------
    // 6. Creator Unlock PIN Lockout Handler
    // --------------------------------------------------------------------------
    if (unlockRoomBtn) {
      unlockRoomBtn.addEventListener('click', async () => {
        unlockRoomBtn.disabled = true;
        unlockRoomBtn.textContent = 'Resetting lockout…';
        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/unlock`, {
            method: 'POST',
          });
          if (res.ok) {
            if (lockoutAlert) lockoutAlert.style.display = 'none';
          } else {
            alert('Failed to reset PIN lockout');
          }
        } catch (e) {
          alert('Network error while resetting lockout');
        } finally {
          unlockRoomBtn.disabled = false;
          unlockRoomBtn.textContent = 'Reset PIN Lockout';
        }
      });
    }

    // --------------------------------------------------------------------------
    // 7. Close Room Handler (Creator Page)
    // --------------------------------------------------------------------------
    const closeBtn = document.getElementById('close-room-btn');
    if (closeBtn) {
      closeBtn.addEventListener('click', async () => {
        if (!confirm('Are you sure you want to close this room? Participants will be disconnected immediately and all temporary files will be purged.')) {
          return;
        }
        closeBtn.disabled = true;
        closeBtn.textContent = 'Closing room…';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/close`, {
            method: 'POST',
          });
          if (res.ok || res.status === 404 || res.status === 410) {
            showInactive('Room Closed', 'You have closed this temporary room.');
          } else {
            const errData = await res.json().catch(() => ({}));
            alert(errData.error || 'Failed to close room');
            closeBtn.disabled = false;
            closeBtn.textContent = 'Close Room Now';
          }
        } catch (err) {
          alert('Network error while closing room');
          closeBtn.disabled = false;
          closeBtn.textContent = 'Close Room Now';
        }
      });
    }

    // --------------------------------------------------------------------------
    // 8. Global Share Creator Handlers
    // --------------------------------------------------------------------------
    const shareModal = document.getElementById('share-modal');
    const closeModalBtn = document.getElementById('close-modal-btn');
    const createShareForm = document.getElementById('create-share-form');
    const modalFileId = document.getElementById('modal-file-id');
    const modalSubtitle = document.getElementById('modal-file-subtitle');
    const shareResultBox = document.getElementById('share-result-box');
    const generatedShareInput = document.getElementById('generated-share-input');
    const copyShareBtn = document.getElementById('copy-share-btn');
    const copyShareToast = document.getElementById('copy-share-toast');
    const shareErrorBox = document.getElementById('share-error-box');

    // Open share modal
    document.addEventListener('click', (e) => {
      if (e.target && e.target.classList.contains('btn-share-link')) {
        const fileId = e.target.dataset.fileId;
        const fileName = e.target.dataset.fileName;
        if (modalFileId) modalFileId.value = fileId;
        if (modalSubtitle) modalSubtitle.textContent = `Generate a temporary, public download link for "${fileName}".`;
        if (shareResultBox) shareResultBox.style.display = 'none';
        if (shareErrorBox) shareErrorBox.style.display = 'none';
        if (shareModal) shareModal.style.display = 'flex';
      }
    });

    if (closeModalBtn && shareModal) {
      closeModalBtn.addEventListener('click', () => {
        shareModal.style.display = 'none';
      });
      shareModal.addEventListener('click', (e) => {
        if (e.target === shareModal) {
          shareModal.style.display = 'none';
        }
      });
    }

    if (createShareForm) {
      createShareForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const fileId = modalFileId ? modalFileId.value : '';
        const ttlSelect = document.getElementById('share-ttl-select');
        const ttlVal = ttlSelect ? parseInt(ttlSelect.value, 10) : 3600;
        const generateBtn = document.getElementById('generate-share-btn');

        if (!fileId) return;

        if (generateBtn) {
          generateBtn.disabled = true;
          generateBtn.textContent = 'Generating Link…';
        }
        if (shareErrorBox) shareErrorBox.style.display = 'none';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/files/${encodeURIComponent(fileId)}/share`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ttl_seconds: ttlVal }),
          });

          const data = await res.json();
          if (res.ok) {
            if (generatedShareInput) generatedShareInput.value = data.share_url;
            if (shareResultBox) shareResultBox.style.display = 'block';
            pollStatus();
          } else {
            if (shareErrorBox) {
              shareErrorBox.textContent = data.error || 'Failed to create share link';
              shareErrorBox.style.display = 'block';
            }
          }
        } catch (err) {
          if (shareErrorBox) {
            shareErrorBox.textContent = 'Network error while creating share link';
            shareErrorBox.style.display = 'block';
          }
        } finally {
          if (generateBtn) {
            generateBtn.disabled = false;
            generateBtn.textContent = 'Create Share Link';
          }
        }
      });
    }

    if (copyShareBtn && generatedShareInput) {
      copyShareBtn.addEventListener('click', async () => {
        try {
          if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(generatedShareInput.value);
          } else {
            generatedShareInput.select();
            document.execCommand('copy');
          }
          if (copyShareToast) {
            copyShareToast.classList.add('show');
            setTimeout(() => copyShareToast.classList.remove('show'), 2500);
          }
        } catch (err) {
          generatedShareInput.select();
        }
      });
    }

    // Revoke share handler
    document.addEventListener('click', async (e) => {
      if (e.target && e.target.classList.contains('btn-revoke-share')) {
        const shareId = e.target.dataset.shareId;
        if (!shareId) return;

        if (!confirm('Are you sure you want to revoke this public share link immediately?')) {
          return;
        }

        e.target.disabled = true;
        e.target.textContent = 'Revoking…';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/shares/${encodeURIComponent(shareId)}/revoke`, {
            method: 'POST',
          });

          if (res.ok) {
            const item = e.target.closest('.share-item');
            if (item) item.remove();
            const shareCountEl = document.getElementById('share-count');
            if (shareCountEl) {
              const current = parseInt(shareCountEl.textContent, 10) || 1;
              shareCountEl.textContent = Math.max(0, current - 1);
            }
          } else {
            const errData = await res.json().catch(() => ({}));
            alert(errData.error || 'Failed to revoke share link');
            e.target.disabled = false;
            e.target.textContent = 'Revoke';
          }
        } catch (err) {
          alert('Network error while revoking share link');
          e.target.disabled = false;
          e.target.textContent = 'Revoke';
        }
      }
    });
  }
})();
