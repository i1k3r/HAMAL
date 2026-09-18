(() => {
  // --------------------------------------------------------------------------
  // Theme Management (Light / Dark Mode with LocalStorage Persistence)
  // --------------------------------------------------------------------------
  function initTheme() {
    const saved = localStorage.getItem('hamal_theme');
    const currentTheme = saved || 'dark';

    document.documentElement.setAttribute('data-theme', currentTheme);
    updateThemeButtons(currentTheme);

    const btnLight = document.getElementById('theme-btn-light');
    const btnDark = document.getElementById('theme-btn-dark');
    const toggleBtn = document.getElementById('theme-toggle-btn');

    if (btnLight) {
      btnLight.addEventListener('click', () => setTheme('light'));
    }
    if (btnDark) {
      btnDark.addEventListener('click', () => setTheme('dark'));
    }
    if (toggleBtn) {
      toggleBtn.addEventListener('click', () => {
        const theme = document.documentElement.getAttribute('data-theme') || 'dark';
        setTheme(theme === 'dark' ? 'light' : 'dark');
      });
    }
  }

  function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('hamal_theme', theme);
    updateThemeButtons(theme);
  }

  function updateThemeButtons(theme) {
    const btnLight = document.getElementById('theme-btn-light');
    const btnDark = document.getElementById('theme-btn-dark');
    if (btnLight && btnDark) {
      if (theme === 'light') {
        btnLight.classList.add('active');
        btnDark.classList.remove('active');
      } else {
        btnDark.classList.add('active');
        btnLight.classList.remove('active');
      }
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initTheme);
  } else {
    initTheme();
  }

  // --------------------------------------------------------------------------
  // Modals Common Controller
  // --------------------------------------------------------------------------
  function showModal(modalId) {
    const backdrop = document.getElementById('modal-backdrop');
    const modal = document.getElementById(modalId);
    if (!backdrop || !modal) return;

    const dialogs = backdrop.querySelectorAll('.modal-dialog');
    dialogs.forEach((d) => d.classList.add('hidden'));

    modal.classList.remove('hidden');
    backdrop.classList.remove('hidden');
  }

  function closeAllModals() {
    const backdrop = document.getElementById('modal-backdrop');
    if (!backdrop) return;
    const dialogs = backdrop.querySelectorAll('.modal-dialog');
    dialogs.forEach((d) => d.classList.add('hidden'));
    backdrop.classList.add('hidden');
  }

  const modalBackdrop = document.getElementById('modal-backdrop');
  if (modalBackdrop) {
    modalBackdrop.addEventListener('click', (e) => {
      if (e.target === modalBackdrop) {
        closeAllModals();
      }
    });
  }

  const navBtnAbout = document.getElementById('nav-btn-about');
  if (navBtnAbout) {
    navBtnAbout.addEventListener('click', () => showModal('modal-about'));
  }
  const aboutClose = document.getElementById('modal-about-close');
  if (aboutClose) {
    aboutClose.addEventListener('click', closeAllModals);
  }
  const aboutDismiss = document.getElementById('modal-about-dismiss');
  if (aboutDismiss) {
    aboutDismiss.addEventListener('click', closeAllModals);
  }

  // --------------------------------------------------------------------------
  // 1. Room Creation Form Handler (Home Page)
  // --------------------------------------------------------------------------
  const createForm = document.getElementById('create-room-form');
  if (createForm) {
    createForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const createBtn = document.getElementById('create-btn') || document.getElementById('create-submit-btn');
      const formError = document.getElementById('form-error') || document.getElementById('create-room-error');
      const ttlSelect = document.getElementById('ttl-select') || document.getElementById('room-ttl-select');
      const pinInput = document.getElementById('pin-input') || document.getElementById('room-pin-input');
      const ttlSeconds = parseInt(ttlSelect ? ttlSelect.value : '3600', 10);
      const pin = pinInput ? pinInput.value.trim() : '';

      if (pin.length > 0 && (pin.length < 4 || pin.length > 8)) {
        if (formError) {
          formError.textContent = 'PIN must be between 4 and 8 characters';
          formError.style.display = 'block';
        }
        return;
      }

      if (createBtn) {
        createBtn.disabled = true;
        createBtn.innerHTML = '<span>Creating room…</span>';
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
          createBtn.innerHTML = `
            <span class="btn-icon-circle">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                <line x1="12" y1="5" x2="12" y2="19"></line>
                <line x1="5" y1="12" x2="19" y2="12"></line>
              </svg>
            </span>
            <span class="btn-text">Create Transfer Room</span>
            <svg class="btn-arrow" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <line x1="5" y1="12" x2="19" y2="12"></line>
              <polyline points="12 5 19 12 12 19"></polyline>
            </svg>
          `;
        }
      }
    });

    const pinToggleBtn = document.getElementById('pin-toggle-btn');
    const pinInput = document.getElementById('pin-input');
    const pinEyeIcon = document.getElementById('pin-eye-icon');
    if (pinToggleBtn && pinInput) {
      pinToggleBtn.addEventListener('click', () => {
        const isPassword = pinInput.getAttribute('type') === 'password';
        pinInput.setAttribute('type', isPassword ? 'text' : 'password');
        if (pinEyeIcon) {
          if (isPassword) {
            // Show eye-off icon
            pinEyeIcon.innerHTML = `
              <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path>
              <line x1="1" y1="1" x2="23" y2="23"></line>
            `;
          } else {
            // Show eye icon
            pinEyeIcon.innerHTML = `
              <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
              <circle cx="12" cy="12" r="3"></circle>
            `;
          }
        }
      });
    }
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
    const pinCard = document.getElementById('pin-barrier-card') || document.getElementById('room-pin-card');
    const inactiveTitle = document.getElementById('inactive-title');
    const inactiveMsg = document.getElementById('inactive-message');

    let isTerminated = false;
    let pollTimer = null;
    const recentActivities = [];

    // Session storage for tracking downloaded files
    const downloadedKey = `hamal_downloaded_${token}`;
    let downloadedFiles = new Set();
    try {
      const stored = sessionStorage.getItem(downloadedKey);
      if (stored) {
        downloadedFiles = new Set(JSON.parse(stored));
      }
    } catch (e) {}

    function markFileDownloaded(fileId, filename) {
      downloadedFiles.add(fileId);
      try {
        sessionStorage.setItem(downloadedKey, JSON.stringify(Array.from(downloadedFiles)));
      } catch (e) {}
      if (filename) {
        addRecentActivity('download', filename, 'Downloaded to device');
      }
    }

    function formatTime(totalSeconds) {
      if (totalSeconds <= 0) return '00:00';
      const hours = Math.floor(totalSeconds / 3600);
      const minutes = Math.floor((totalSeconds % 3600) / 60);
      const seconds = totalSeconds % 60;

      const pad = (n) => String(n).padStart(2, '0');
      if (hours > 0) {
        return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
      }
      return `${pad(minutes)}:${pad(seconds)}`;
    }

    function formatBytes(bytes) {
      if (!bytes || bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    // 20 Languages Translation Map for Participant & Executable Warnings & Text Sharing
    const PARTICIPANT_I18N = {
      en: {
        executableWarning: "Potentially executable file. Only open or install files you trust.",
        closeBtn: "Close Room",
        confirmTitle: "Close this transfer room?",
        confirmDesc: "All temporary files will be purged and participants cannot rejoin.",
        cancelBtn: "Cancel",
        confirmBtn: "Close Room",
        closing: "Closing room…",
        closedTitle: "Room Closed",
        closedMsg: "This temporary transfer room has been closed.",
        closeError: "Failed to close room",
        networkError: "Network error while closing room",
        roomClosing: "ROOM CLOSING",
        closingDesc: "This transfer room will close automatically in",
        seconds: "seconds",
        filesTab: "Files",
        textTab: "Text",
        sendText: "Send",
        sendingText: "Sending…",
        copyText: "Copy",
        copiedText: "Copied!",
        noTextsYet: "No shared text yet.",
        noTextsSub: "Paste commands, logs, snippets, or URLs to share directly.",
        textPlaceholder: "Paste or type a message, command, log, code snippet, or note…",
        textTooLarge: "Text exceeds maximum allowed size",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "New message"
      },
      tr: {
        executableWarning: "Potansiyel olarak çalıştırılabilir dosya. Yalnızca güvendiğiniz dosyaları açın veya yükleyin.",
        closeBtn: "ODAYI ŞİMDİ KAPAT",
        confirmTitle: "Bu transfer odası kapatılsın mı?",
        confirmDesc: "Tüm geçici dosyalar silinecek ve katılımcılar tekrar bağlanamayacaktır.",
        cancelBtn: "İptal",
        confirmBtn: "Odayı Kapat",
        closing: "Oda kapatılıyor…",
        closedTitle: "Oda Kapatıldı",
        closedMsg: "Bu geçici transfer odası kapatıldı.",
        closeError: "Oda kapatılamadı",
        networkError: "Oda kapatılırken ağ hatası oluştu",
        roomClosing: "ODA KAPATILIYOR",
        closingDesc: "Bu transfer odası otomatik olarak kapanacak:",
        seconds: "saniye",
        filesTab: "Dosyalar",
        textTab: "Metin",
        sendText: "Gönder",
        sendingText: "Gönderiliyor…",
        copyText: "Kopyala",
        copiedText: "Kopyalandı!",
        noTextsYet: "Henüz paylaşılan metin yok.",
        noTextsSub: "Doğrudan paylaşmak için komut, günlük, kod parçacığı veya bağlantı yapıştırın.",
        textPlaceholder: "Mesaj, komut, günlük, kod veya not yapıştırın ya da yazın…",
        textTooLarge: "Metin izin verilen maksimum boyutu aşıyor",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Yeni mesaj"
      },
      "zh-CN": {
        executableWarning: "可能为可执行文件。请仅打开或安装您信任的文件。",
        closeBtn: "关闭房间",
        confirmTitle: "确定关闭此传输房间？",
        confirmDesc: "所有临时文件将被清除，参与者将无法重新加入。",
        cancelBtn: "取消",
        confirmBtn: "关闭房间",
        closing: "正在关闭房间…",
        closedTitle: "房间已关闭",
        closedMsg: "此临时传输房间已关闭。",
        closeError: "关闭房间失败",
        networkError: "关闭房间时发生网络错误",
        roomClosing: "房间即将关闭",
        closingDesc: "此传输房间将在以下时间内自动关闭：",
        seconds: "秒",
        filesTab: "文件",
        textTab: "文本",
        sendText: "发送",
        sendingText: "正在发送…",
        copyText: "复制",
        copiedText: "已复制！",
        noTextsYet: "暂无共享文本。",
        noTextsSub: "直接粘贴命令、日志、代码片段或链接进行共享。",
        textPlaceholder: "粘贴或输入消息、命令、日志、代码或备注…",
        textTooLarge: "文本超出允许的最大大小",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "新消息"
      },
      hi: {
        executableWarning: "संभावित रूप से निष्पादन योग्य फ़ाइल। केवल उन्हीं फ़ाइलों को खोलें या इंस्टॉल करें जिन पर आप भरोसा करते हैं।",
        closeBtn: "कमरा बंद करें",
        confirmTitle: "क्या आप यह ट्रांसफर रूम बंद करना चाहते हैं?",
        confirmDesc: "सभी अस्थायी फ़ाइलें हटा दी जाएंगी और प्रतिभागी दोबारा नहीं जुड़ सकेंगे।",
        cancelBtn: "रद्द करें",
        confirmBtn: "कमरा बंद करें",
        closing: "कमरा बंद हो रहा है…",
        closedTitle: "कमरा बंद है",
        closedMsg: "यह अस्थायी ट्रांसफर रूम बंद कर दिया गया है।",
        closeError: "कमरा बंद करने में विफल",
        networkError: "कमरा बंद करते समय नेटवर्क त्रुटि",
        roomClosing: "कमरा बंद हो रहा है",
        closingDesc: "यह ट्रांसफर रूम स्वचालित रूप से बंद हो जाएगा:",
        seconds: "सेकंड",
        filesTab: "फ़ाइलें",
        textTab: "पाठ",
        sendText: "भेजें",
        sendingText: "भेजा जा रहा है…",
        copyText: "कॉपी करें",
        copiedText: "कॉपी हो गया!",
        noTextsYet: "अभी तक कोई साझा पाठ नहीं है।",
        noTextsSub: "कमांड, लॉग, कोड स्निपेट या लिंक पेस्ट करें।",
        textPlaceholder: "संदेश, कमांड, लॉग, कोड या नोट पेस्ट या टाइप करें…",
        textTooLarge: "पाठ अनुमत अधिकतम आकार से अधिक है",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "नया संदेश"
      },
      es: {
        executableWarning: "Archivo potencialmente ejecutable. Solo abra o instale archivos de confianza.",
        closeBtn: "Cerrar sala",
        confirmTitle: "¿Cerrar esta sala de transferencia?",
        confirmDesc: "Se eliminarán todos los archivos temporales y los participantes no podrán volver a unirse.",
        cancelBtn: "Cancelar",
        confirmBtn: "Cerrar sala",
        closing: "Cerrando sala…",
        closedTitle: "Sala cerrada",
        closedMsg: "Esta sala temporal de transferencia ha sido cerrada.",
        closeError: "Error al cerrar la sala",
        networkError: "Error de red al cerrar la sala",
        roomClosing: "CERRANDO SALA",
        closingDesc: "Esta sala de transferencia se cerrará automáticamente en",
        seconds: "segundos",
        filesTab: "Archivos",
        textTab: "Texto",
        sendText: "Enviar",
        sendingText: "Enviando…",
        copyText: "Copiar",
        copiedText: "¡Copiado!",
        noTextsYet: "Aún no hay texto compartido.",
        noTextsSub: "Pega comandos, registros, fragmentos o enlaces para compartir directamente.",
        textPlaceholder: "Pega o escribe un mensaje, comando, registro, código o nota…",
        textTooLarge: "El texto excede el tamaño máximo permitido",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nuevo mensaje"
      },
      fr: {
        executableWarning: "Fichier potentiellement exécutable. N'ouvrez ou n'installez que des fichiers de confiance.",
        closeBtn: "Fermer le salon",
        confirmTitle: "Fermer ce salon de transfert ?",
        confirmDesc: "Tous les fichiers temporaires seront supprimés et les participants ne pourront plus se reconnecter.",
        cancelBtn: "Annuler",
        confirmBtn: "Fermer le salon",
        closing: "Fermeture du salon…",
        closedTitle: "Salon fermé",
        closedMsg: "Ce salon de transfert temporaire a été fermé.",
        closeError: "Échec de la fermeture du salon",
        networkError: "Erreur réseau lors de la fermeture du salon",
        roomClosing: "FERMETURE DU SALON",
        closingDesc: "Ce salon de transfert fermera automatiquement dans",
        seconds: "secondes",
        filesTab: "Fichiers",
        textTab: "Texte",
        sendText: "Envoyer",
        sendingText: "Envoi en cours…",
        copyText: "Copier",
        copiedText: "Copié !",
        noTextsYet: "Aucun texte partagé pour le moment.",
        noTextsSub: "Collez des commandes, journaux, extraits ou liens à partager directement.",
        textPlaceholder: "Collez ou écrivez un message, commande, journal, code ou note…",
        textTooLarge: "Le texte dépasse la taille maximale autorisée",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nouveau message"
      },
      ar: {
        executableWarning: "ملف قابل للتنفيذ المحتمل. افتح أو ثبّت فقط الملفات التي تثق بها.",
        closeBtn: "إغلاق الغرفة",
        confirmTitle: "هل تريد إغلاق غرفة النقل هذه؟",
        confirmDesc: "سيتم حذف جميع الملفات المؤقتة ولن يتمكن المشاركون من الانضمام مجددًا.",
        cancelBtn: "إلغاء",
        confirmBtn: "إغلاق الغرفة",
        closing: "جارٍ إغلاق الغرفة…",
        closedTitle: "تم إغلاق الغرفة",
        closedMsg: "تم إغلاق غرفة النقل المؤقتة هذه.",
        closeError: "فشل إغلاق الغرفة",
        networkError: "خطأ في الشبكة أثناء إغلاق الغرفة",
        roomClosing: "جارٍ إغلاق الغرفة",
        closingDesc: "ستُغلق غرفة النقل هذه تلقائيًا خلال",
        seconds: "ثوانٍ",
        filesTab: "الملفات",
        textTab: "النص",
        sendText: "إرسال",
        sendingText: "جارٍ الإرسال…",
        copyText: "نسخ",
        copiedText: "تم النسخ!",
        noTextsYet: "لا يوجد نص مشترك حتى الآن.",
        noTextsSub: "الصق الأوامر، السجلات، المقتطفات أو الروابط للمشاركة مباشرة.",
        textPlaceholder: "الصق أو اكتب رسالة، أمرًا، سجلًا، رمزًا أو ملاحظة…",
        textTooLarge: "يتجاوز النص الحد الأقصى للحجم المسموح به",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "رسالة جديدة"
      },
      bn: {
        executableWarning: "সম্ভাব্য এক্সিকিউটেবল ফাইল। শুধুমাত্র আপনার বিশ্বস্ত ফাইল খুলুন বা ইনস্টল করুন।",
        closeBtn: "রুম বন্ধ করুন",
        confirmTitle: "এই ট্রান্সফার রুমটি বন্ধ করবেন?",
        confirmDesc: "সমস্ত অস্থায়ী ফাইল মুছে ফেলা হবে এবং অংশগ্রহণকারীরা পুনরায় যোগ দিতে পারবেন না।",
        cancelBtn: "বাতিল",
        confirmBtn: "রুম বন্ধ করুন",
        closing: "রুম বন্ধ হচ্ছে…",
        closedTitle: "রুম বন্ধ",
        closedMsg: "এই অস্থায়ী ট্রান্সফার রুমটি বন্ধ করা হয়েছে।",
        closeError: "রুম বন্ধ করতে ব্যর্থ",
        networkError: "রুম বন্ধ করার সময় নেটওয়ার্ক ত্রুটি",
        roomClosing: "রুম বন্ধ হচ্ছে",
        closingDesc: "এই ট্রান্সফার রুমটি স্বয়ংক্রিয়ভাবে বন্ধ হয়ে যাবে:",
        seconds: "সেকেন্ড",
        filesTab: "ফাইল",
        textTab: "টেক্সট",
        sendText: "পাঠান",
        sendingText: "পাঠানো হচ্ছে…",
        copyText: "কপি করুন",
        copiedText: "কপি হয়েছে!",
        noTextsYet: "এখনো কোনো টেক্সট শেয়ার করা হয়নি।",
        noTextsSub: "সরাসরি শেয়ার করতে কমান্ড, লগ, কোড বা লিঙ্ক পেস্ট করুন।",
        textPlaceholder: "বার্তা, কমান্ড, লগ, কোড বা নোট পেস্ট বা টাইপ করুন…",
        textTooLarge: "টেক্সটের আকার অনুমোদিত সীমা অতিক্রম করেছে",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "নতুন বার্তা"
      },
      pt: {
        executableWarning: "Arquivo potencialmente executável. Apenas abra ou instale arquivos confiáveis.",
        closeBtn: "Fechar sala",
        confirmTitle: "Fechar esta sala de transferência?",
        confirmDesc: "Todos os arquivos temporários serão apagados e os participantes não poderão retornar.",
        cancelBtn: "Cancelar",
        confirmBtn: "Fechar sala",
        closing: "Fechando sala…",
        closedTitle: "Sala encerrada",
        closedMsg: "Esta sala de transferência temporária foi encerrada.",
        closeError: "Falha ao fechar a sala",
        networkError: "Erro de rede ao fechar a sala",
        roomClosing: "FECHANDO SALA",
        closingDesc: "Esta sala de transferência fechará automaticamente em",
        seconds: "segundos",
        filesTab: "Arquivos",
        textTab: "Texto",
        sendText: "Enviar",
        sendingText: "Enviando…",
        copyText: "Copiar",
        copiedText: "Copiado!",
        noTextsYet: "Nenhum texto compartilhado ainda.",
        noTextsSub: "Cole comandos, registros, snippets ou links para compartilhar diretamente.",
        textPlaceholder: "Cole ou digite uma mensagem, comando, log, código ou nota…",
        textTooLarge: "O texto excede o tamanho máximo permitido",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nova mensagem"
      },
      ru: {
        executableWarning: "Потенциально исполняемый файл. Открывайте и устанавливайте только файлы, которым доверяете.",
        closeBtn: "Закрыть комнату",
        confirmTitle: "Закрыть эту комнату передачи?",
        confirmDesc: "Все временные файлы будут удалены, а участники не смогут присоединиться снова.",
        cancelBtn: "Отмена",
        confirmBtn: "Закрыть комнату",
        closing: "Закрытие комнаты…",
        closedTitle: "Комната закрыта",
        closedMsg: "Эта временная комната передачи была закрыта.",
        closeError: "Не удалось закрыть комнату",
        networkError: "Сетевая ошибка при закрытии комнаты",
        roomClosing: "ЗАКРЫТИЕ КОМНАТЫ",
        closingDesc: "Эта комната передачи закроется автоматически через",
        seconds: "сек.",
        filesTab: "Файлы",
        textTab: "Текст",
        sendText: "Отправить",
        sendingText: "Отправка…",
        copyText: "Копировать",
        copiedText: "Скопировано!",
        noTextsYet: "Пока нет сообщений.",
        noTextsSub: "Вставьте команды, логи, код или ссылки для быстрой передачи.",
        textPlaceholder: "Вставьте или введите сообщение, команду, лог, код или заметку…",
        textTooLarge: "Размер текста превышает максимально допустимый",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Новое сообщение"
      },
      ur: {
        executableWarning: "ممکنہ طور پر قابل عمل فائل۔ صرف ان فائلوں کو کھولیں یا انسٹال کریں جن پر آپ کو بھروسہ ہو۔",
        closeBtn: "کمرہ بند کریں",
        confirmTitle: "کیا آپ یہ ٹرانسفر روم بند کرنا چاہتے ہیں؟",
        confirmDesc: "تمام عارضی فائلیں خارج کر دی جائیں گی اور شرکاء دوبارہ شامل نہیں ہو سکیں گے۔",
        cancelBtn: "منسوخ",
        confirmBtn: "کمرہ بند کریں",
        closing: "کمرہ بند ہو رہا ہے…",
        closedTitle: "کمرہ بند ہے",
        closedMsg: "یہ عارضی ٹرانسفر روم بند کر دیا گیا ہے۔",
        closeError: "کمرہ بند کرنے میں ناکامی",
        networkError: "کمرہ بند کرتے وقت نیٹ ورک خرابی",
        roomClosing: "کمرہ بند ہو رہا ہے",
        closingDesc: "یہ ٹرانسفر روم خودکار طریقے سے بند ہو جائے گا:",
        seconds: "سیکنڈ",
        filesTab: "فائلیں",
        textTab: "متن",
        sendText: "ارسال کریں",
        sendingText: "بھیجا جا رہا ہے…",
        copyText: "کاپی کریں",
        copiedText: "کاپی ہو گیا!",
        noTextsYet: "ابھی تک کوئی مشترکہ متن نہیں ہے۔",
        noTextsSub: "براہ راست شیئر کرنے کے لیے کمانڈز، لاگز، کوڈ یا لنکس پیسٹ کریں۔",
        textPlaceholder: "پیغام، کمانڈ، لاگ، کوڈ یا نوٹ پیسٹ یا ٹائپ کریں…",
        textTooLarge: "متن اجازت شدہ زیادہ سے زیادہ سائز سے زیادہ ہے",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "نیا پیغام"
      },
      id: {
        executableWarning: "File yang berpotensi dapat dieksekusi. Hanya buka atau instal file yang Anda percayai.",
        closeBtn: "Tutup Ruangan",
        confirmTitle: "Tutup ruangan transfer ini?",
        confirmDesc: "Semua file sementara akan dihapus dan peserta tidak dapat bergabung kembali.",
        cancelBtn: "Batal",
        confirmBtn: "Tutup Ruangan",
        closing: "Menutup ruangan…",
        closedTitle: "Ruangan Ditutup",
        closedMsg: "Ruangan transfer sementara ini telah ditutup.",
        closeError: "Gagal menutup ruangan",
        networkError: "Kesalahan jaringan saat menutup ruangan",
        roomClosing: "MENUTUP RUANGAN",
        closingDesc: "Ruangan transfer ini akan ditutup secara otomatis dalam",
        seconds: "detik",
        filesTab: "File",
        textTab: "Teks",
        sendText: "Kirim",
        sendingText: "Mengirim…",
        copyText: "Salin",
        copiedText: "Tersalin!",
        noTextsYet: "Belum ada teks yang dibagikan.",
        noTextsSub: "Tempel perintah, log, cuplikan kode, atau tautan untuk dibagikan langsung.",
        textPlaceholder: "Tempel atau ketik pesan, perintah, log, kode, atau catatan…",
        textTooLarge: "Teks melebihi ukuran maksimum yang diizinkan",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Pesan baru"
      },
      de: {
        executableWarning: "Potenziell ausführbare Datei. Öffnen oder installieren Sie nur Dateien, denen Sie vertrauen.",
        closeBtn: "Raum schließen",
        confirmTitle: "Diesen Übertragungsraum schließen?",
        confirmDesc: "Alle temporären Dateien werden gelöscht und Teilnehmer können nicht mehr beitreten.",
        cancelBtn: "Abbrechen",
        confirmBtn: "Raum schließen",
        closing: "Raum wird geschlossen…",
        closedTitle: "Raum geschlossen",
        closedMsg: "Dieser temporäre Übertragungsraum wurde geschlossen.",
        closeError: "Fehler beim Schließen des Raums",
        networkError: "Netzwerkfehler beim Schließen des Raums",
        roomClosing: "RAUM WIRD GESCHLOSSEN",
        closingDesc: "Dieser Übertragungsraum wird automatisch geschlossen in",
        seconds: "Sekunden",
        filesTab: "Dateien",
        textTab: "Text",
        sendText: "Senden",
        sendingText: "Wird gesendet…",
        copyText: "Kopieren",
        copiedText: "Kopiert!",
        noTextsYet: "Noch kein geteilter Text.",
        noTextsSub: "Fügen Sie Befehle, Protokolle, Code oder Links zum direkten Teilen ein.",
        textPlaceholder: "Fügen Sie eine Nachricht, einen Befehl, Protokolle, Code oder eine Notiz ein…",
        textTooLarge: "Text überschreitet die maximal zulässige Größe",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Neue Nachricht"
      },
      ja: {
        executableWarning: "実行可能ファイルの可能性があります。信頼できるファイルのみを開くかインストールしてください。",
        closeBtn: "ルームを閉じる",
        confirmTitle: "この転送ルームを閉じますか？",
        confirmDesc: "すべての一時ファイルが削除され、参加者は再接続できなくなります。",
        cancelBtn: "キャンセル",
        confirmBtn: "ルームを閉じる",
        closing: "ルームを閉じています…",
        closedTitle: "ルームは閉じられました",
        closedMsg: "この一時転送ルームは終了しました。",
        closeError: "ルームの終了に失敗しました",
        networkError: "ルーム終了中にネットワークエラーが発生しました",
        roomClosing: "ルームを終了中",
        closingDesc: "この転送ルームは自動的に終了します:",
        seconds: "秒",
        filesTab: "ファイル",
        textTab: "テキスト",
        sendText: "送信",
        sendingText: "送信中…",
        copyText: "コピー",
        copiedText: "コピーしました！",
        noTextsYet: "共有されたテキストはまだありません。",
        noTextsSub: "コマンド、ログ、コード、リンクを貼り付けて直接共有できます。",
        textPlaceholder: "メッセージ、コマンド、ログ、コード、メモを貼り付けるか入力…",
        textTooLarge: "テキストが許可された最大サイズを超えています",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "新着メッセージ"
      },
      mr: {
        executableWarning: "संभाव्य एक्झिक्युटेबल फाइल. फक्त तुमच्या विश्वासू फाइल्स उघडा किंवा इन्स्टॉल करा.",
        closeBtn: "रूम बंद करा",
        confirmTitle: "ही ट्रान्सफर रूम बंद करायची?",
        confirmDesc: "सर्व तात्पुरत्या फाइल्स नष्ट केल्या जातील आणि सहभागी पुन्हा कनेक्ट होऊ शकणार नाहीत.",
        cancelBtn: "रद्द करा",
        confirmBtn: "रूम बंद करा",
        closing: "रूम बंद होत आहे…",
        closedTitle: "रूम बंद झाली",
        closedMsg: "ही तात्पुरती ट्रान्सफर रूम बंद करण्यात आली आहे.",
        closeError: "रूम बंद करणे अयशस्वी",
        networkError: "रूम बंद करताना नेटवर्क त्रुटी",
        roomClosing: "रूम बंद होत आहे",
        closingDesc: "हा ट्रान्सफर रूम आपोआप बंद होईल:",
        seconds: "सेकंद",
        filesTab: "फायली",
        textTab: "मजकूर",
        sendText: "पाठवा",
        sendingText: "पाठवत आहे…",
        copyText: "कॉपी करा",
        copiedText: "कॉपी केले!",
        noTextsYet: "अद्याप कोणताही शेअर केलेला मजकूर नाही.",
        noTextsSub: "थेट शेअर करण्यासाठी कमांड्स, लॉग्स, कोड किंवा लिंक्स पेस्ट करा.",
        textPlaceholder: "संदेश, कमांड, लॉग, कोड स्निपेट किंवा नोंद पेस्ट किंवा टाइप करा…",
        textTooLarge: "मजकूर कमाल परवानगी असलेल्या आकारापेक्षा मोठा आहे",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "नवीन संदेश"
      },
      te: {
        executableWarning: "సంభావ్య ఎక్జిక్యూటబుల్ ఫైల్. మీరు విశ్వసించే ఫైల్‌లను మాత్రమే తెరవండి లేదా ఇన్‌స్టాల్ చేయండి.",
        closeBtn: "గదిని మూసివేయి",
        confirmTitle: "ఈ బదిలీ గదిని మూసివేయాలా?",
        confirmDesc: "అన్ని తాత్కాలిక ఫైల్‌లు తొలగించబడతాయి మరియు పాల్గొనేవారు మళ్లీ చేరలేరు.",
        cancelBtn: "రద్దు చేయి",
        confirmBtn: "గదిని మూసివేయి",
        closing: "గది మూసివేయబడుతోంది…",
        closedTitle: "గది మూసివేయబడింది",
        closedMsg: "ఈ తాత్కాలిక బదిలీ గది మూసివేయబడింది.",
        closeError: "గదిని మూసివేయడం విఫలమైంది",
        networkError: "గదిని మూసివేసేటప్పుడు నెట్‌వర్క్ లోపం",
        roomClosing: "గది మూసివేయబడుతోంది",
        closingDesc: "ఈ బదిలీ గది స్వయంచాలకంగా మూసివేయబడుతుంది:",
        seconds: "సెకన్లు",
        filesTab: "ఫైల్‌లు",
        textTab: "టెక్స్ట్",
        sendText: "పంపు",
        sendingText: "పంపుతోంది…",
        copyText: "కాపీ చేయి",
        copiedText: "కాపీ చేయబడింది!",
        noTextsYet: "ఇంకా భాగస్వామ్య టెక్స్ట్ లేదు.",
        noTextsSub: "కమాండ్‌లు, లాగ్‌లు, కోడ్ లేదా లింక్‌లను నేరుగా షేర్ చేయడానికి అతికించండి.",
        textPlaceholder: "సందేశం, కమాండ్, లాగ్, కోడ్ లేదా నోట్‌ని అతికించండి లేదా టైప్ చేయండి…",
        textTooLarge: "టెక్స్ట్ అనుమతించబడిన గరిష్ట పరిమాణాన్ని మించిపోయింది",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "కొత్త సందేశం"
      },
      nl: {
        executableWarning: "Mogelijk uitvoerbaar bestand. Open of installeer alleen bestanden die u vertrouwt.",
        closeBtn: "Kamer sluiten",
        confirmTitle: "Deze overdrachtskamer sluiten?",
        confirmDesc: "Alle tijdelijke bestanden worden gewist en deelnemers kunnen niet opnieuw deelnemen.",
        cancelBtn: "Annuleren",
        confirmBtn: "Kamer sluiten",
        closing: "Kamer sluiten…",
        closedTitle: "Kamer gesloten",
        closedMsg: "Deze tijdelijke overdrachtskamer is gesloten.",
        closeError: "Kamer sluiten mislukt",
        networkError: "Netwerkfout bij het sluiten van de kamer",
        roomClosing: "KAMER SLUITEN",
        closingDesc: "Deze overdrachtskamer sluit automatisch over",
        seconds: "seconden",
        filesTab: "Bestanden",
        textTab: "Tekst",
        sendText: "Versturen",
        sendingText: "Versturen…",
        copyText: "Kopiëren",
        copiedText: "Gekopieerd!",
        noTextsYet: "Nog geen gedeelde tekst.",
        noTextsSub: "Plak opdrachten, logs, code of links om direct te delen.",
        textPlaceholder: "Plak of typ een bericht, opdracht, log, codefragment of notitie…",
        textTooLarge: "Tekst overschrijdt de maximaal toegestane grootte",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nieuw bericht"
      },
      it: {
        executableWarning: "File potenzialmente eseguibile. Apri o installa solo i file di cui ti fidi.",
        closeBtn: "Chiudi stanza",
        confirmTitle: "Chiudere questa stanza di trasferimento?",
        confirmDesc: "Tutti i file temporanei saranno eliminati e i partecipanti non potranno rientrare.",
        cancelBtn: "Annulla",
        confirmBtn: "Chiudi stanza",
        closing: "Chiusura stanza…",
        closedTitle: "Stanza chiusa",
        closedMsg: "Questa stanza di trasferimento temporanea è stata chiusa.",
        closeError: "Impossibile chiudere la stanza",
        networkError: "Errore di rete durante la chiusura della stanza",
        roomClosing: "CHIUSURA STANZA",
        closingDesc: "Questa stanza di trasferimento si chiuderà automaticamente tra",
        seconds: "secondi",
        filesTab: "File",
        textTab: "Testo",
        sendText: "Invia",
        sendingText: "Invio in corso…",
        copyText: "Copia",
        copiedText: "Copiato!",
        noTextsYet: "Nessun testo condiviso ancora.",
        noTextsSub: "Incolla comandi, log, frammenti di codice o link da condividere direttamente.",
        textPlaceholder: "Incolla o digita un messaggio, comando, log, codice o nota…",
        textTooLarge: "Il testo supera la dimensione massima consentita",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nuovo messaggio"
      },
      ko: {
        executableWarning: "실행 가능한 파일일 수 있습니다. 신뢰할 수 있는 파일만 열거나 설치하십시오.",
        closeBtn: "방 닫기",
        confirmTitle: "이 전송 방을 닫으시겠습니까?",
        confirmDesc: "모든 임시 파일이 삭제되며 참여자는 다시 참여할 수 없습니다.",
        cancelBtn: "취소",
        confirmBtn: "방 닫기",
        closing: "방 닫는 중…",
        closedTitle: "방 닫힘",
        closedMsg: "이 임시 전송 방이 닫혔습니다.",
        closeError: "방 닫기 실패",
        networkError: "방 닫는 중 네트워크 오류 발생",
        roomClosing: "방 종료 중",
        closingDesc: "이 전송 방은 다음 시간 후에 자동으로 닫힙니다:",
        seconds: "초",
        filesTab: "파일",
        textTab: "텍스트",
        sendText: "보내기",
        sendingText: "전송 중…",
        copyText: "복사",
        copiedText: "복사됨!",
        noTextsYet: "공유된 텍스트가 아직 없습니다.",
        noTextsSub: "명령어, 로그, 코드 조각 또는 링크를 붙여넣어 바로 공유하세요.",
        textPlaceholder: "메시지, 명령어, 로그, 코드 또는 메모를 붙여넣거나 입력하세요…",
        textTooLarge: "텍스트가 허용된 최대 크기를 초과했습니다",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "새 메시지"
      },
      pl: {
        executableWarning: "Plik potencjalnie wykonywalny. Otwieraj lub instaluj wyłącznie pliki, którym ufasz.",
        closeBtn: "Zamknij pokój",
        confirmTitle: "Zamknąć ten pokój transferowy?",
        confirmDesc: "Wszystkie pliki tymczasowe zostaną usunięte, a uczestnicy nie będą mogli dołączyć ponownie.",
        cancelBtn: "Anuluj",
        confirmBtn: "Zamknij pokój",
        closing: "Zamykanie pokoju…",
        closedTitle: "Pokój zamknięty",
        closedMsg: "Ten tymczasowy pokój transferowy został zamknięty.",
        closeError: "Nie udało się zamknąć pokoju",
        networkError: "Błąd sieci podczas zamykania pokoju",
        roomClosing: "ZAMYKANIE POKOJU",
        closingDesc: "Ten pokój transferowy zamknie się automatycznie za",
        seconds: "sekund",
        filesTab: "Pliki",
        textTab: "Tekst",
        sendText: "Wyślij",
        sendingText: "Wysyłanie…",
        copyText: "Kopiuj",
        copiedText: "Skopiowano!",
        noTextsYet: "Brak udostępnionego tekstu.",
        noTextsSub: "Wklej polecenia, dzienniki, fragmenty kodu lub linki do bezpośredniego udostępnienia.",
        textPlaceholder: "Wklej lub wpisz wiadomość, polecenie, dziennik, kod lub notatkę…",
        textTooLarge: "Tekst przekracza maksymalny dozwolony rozmiar",
        client: "CLIENT",
        server: "SERVER",
        newMessage: "Nowa wiadomość"
      }
    };

    function resolveLocaleKey() {
      const langs = navigator.languages || [navigator.language || 'en'];
      for (const lang of langs) {
        if (!lang) continue;
        const low = lang.toLowerCase();
        if (low.startsWith('zh')) return 'zh-CN';
        if (low.startsWith('tr')) return 'tr';
        if (low.startsWith('hi')) return 'hi';
        if (low.startsWith('es')) return 'es';
        if (low.startsWith('fr')) return 'fr';
        if (low.startsWith('ar')) return 'ar';
        if (low.startsWith('bn')) return 'bn';
        if (low.startsWith('pt')) return 'pt';
        if (low.startsWith('ru')) return 'ru';
        if (low.startsWith('ur')) return 'ur';
        if (low.startsWith('id')) return 'id';
        if (low.startsWith('de')) return 'de';
        if (low.startsWith('ja')) return 'ja';
        if (low.startsWith('mr')) return 'mr';
        if (low.startsWith('te')) return 'te';
        if (low.startsWith('nl')) return 'nl';
        if (low.startsWith('it')) return 'it';
        if (low.startsWith('ko')) return 'ko';
        if (low.startsWith('pl')) return 'pl';
        if (low.startsWith('en')) return 'en';
      }
      return 'en';
    }

    const t = PARTICIPANT_I18N[resolveLocaleKey()] || PARTICIPANT_I18N.en;

    // Pixel-Art Category SVGs matching Windows Master Reference
    const PIXEL_ICONS = {
      image: `<svg class="pixel-icon pixel-icon-image" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Image"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg>`,
      video: `<svg class="pixel-icon pixel-icon-video" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Video"><polygon points="23 7 16 12 23 17 23 7"></polygon><rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect></svg>`,
      audio: `<svg class="pixel-icon pixel-icon-audio" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Audio"><path d="M9 18V5l12-2v13"></path><circle cx="6" cy="18" r="3"></circle><circle cx="18" cy="16" r="3"></circle></svg>`,
      pdf: `<svg class="pixel-icon pixel-icon-pdf" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="PDF Document"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M9 15h2a1.5 1.5 0 0 0 0-3H9v6"></path><path d="M15 12h-2v6"></path></svg>`,
      doc: `<svg class="pixel-icon pixel-icon-doc" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Document"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line></svg>`,
      sheet: `<svg class="pixel-icon pixel-icon-sheet" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Spreadsheet"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M8 13h8M8 17h8M12 13v8"></path></svg>`,
      presentation: `<svg class="pixel-icon pixel-icon-pres" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Presentation"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>`,
      archive: `<svg class="pixel-icon pixel-icon-archive" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Archive"><polyline points="21 8 21 21 3 21 3 8"></polyline><rect x="1" y="3" width="22" height="5"></rect><line x1="10" y1="12" x2="14" y2="12"></line></svg>`,
      disk: `<svg class="pixel-icon pixel-icon-disk" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Disk Image"><circle cx="12" cy="12" r="10"></circle><circle cx="12" cy="12" r="3"></circle></svg>`,
      code: `<svg class="pixel-icon pixel-icon-code" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Code"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>`,
      lib: `<svg class="pixel-icon pixel-icon-lib" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Library Component"><polygon points="12 2 2 7 12 12 22 7 12 2"></polygon><polyline points="2 17 12 22 22 17"></polyline><polyline points="2 12 12 17 22 12"></polyline></svg>`,
      win_exe: `<svg class="pixel-icon pixel-icon-exe" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Windows Application"><rect x="3" y="3" width="18" height="18" rx="3" ry="3"></rect><polygon points="10 8 16 12 10 16 10 8"></polygon></svg>`,
      script: `<svg class="pixel-icon pixel-icon-script" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Script"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>`,
      java: `<svg class="pixel-icon pixel-icon-java" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Java Package"><path d="M18 8h1a4 4 0 0 1 0 8h-1"></path><path d="M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8z"></path><line x1="6" y1="1" x2="6" y2="4"></line><line x1="10" y1="1" x2="10" y2="4"></line><line x1="14" y1="1" x2="14" y2="4"></line></svg>`,
      android: `<svg class="pixel-icon pixel-icon-android" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Android Package"><rect x="5" y="2" width="14" height="20" rx="2" ry="2"></rect><line x1="12" y1="18" x2="12.01" y2="18"></line></svg>`,
      linux: `<svg class="pixel-icon pixel-icon-linux" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Linux Application"><rect x="4" y="4" width="16" height="16" rx="2"></rect><circle cx="9" cy="9" r="1"></circle><circle cx="15" cy="9" r="1"></circle><path d="M8 15s1.5 2 4 2 4-2 4-2"></path></svg>`,
      generic: `<svg class="pixel-icon pixel-icon-generic" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="File"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>`
    };

    function getFileCategory(filename) {
      if (!filename) return 'generic';
      const lower = filename.toLowerCase();

      if (lower.endsWith('.tar.gz') || lower.endsWith('.tar.bz2') || lower.endsWith('.tar.xz')) {
        return 'archive';
      }

      const parts = lower.split('.');
      if (parts.length <= 1) return 'generic';
      const ext = parts.pop();

      if (['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp', 'tiff', 'tif', 'svg', 'ico'].includes(ext)) return 'image';
      if (['mp4', 'mkv', 'avi', 'mov', 'webm', 'm4v', 'wmv'].includes(ext)) return 'video';
      if (['mp3', 'wav', 'flac', 'm4a', 'aac', 'ogg', 'wma'].includes(ext)) return 'audio';
      if (ext === 'pdf') return 'pdf';
      if (['doc', 'docx', 'odt', 'rtf', 'txt', 'md'].includes(ext)) return 'doc';
      if (['xls', 'xlsx', 'csv', 'ods'].includes(ext)) return 'sheet';
      if (['ppt', 'pptx', 'odp'].includes(ext)) return 'presentation';
      if (['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz'].includes(ext)) return 'archive';
      if (['iso', 'img'].includes(ext)) return 'disk';
      if (['exe', 'msi', 'com', 'scr'].includes(ext)) return 'win_exe';
      if (['bat', 'cmd', 'vbs', 'ps1', 'sh'].includes(ext)) return 'script';
      if (ext === 'jar') return 'java';
      if (['apk', 'aab'].includes(ext)) return 'android';
      if (['appimage', 'deb', 'rpm'].includes(ext)) return 'linux';
      if (['dll', 'so', 'dylib'].includes(ext)) return 'lib';
      if (['html', 'htm', 'css', 'js', 'ts', 'json', 'xml', 'go', 'py', 'java', 'c', 'cpp', 'h', 'hpp', 'rs'].includes(ext)) return 'code';

      return 'generic';
    }

    function isPotentiallyExecutable(filename) {
      if (!filename) return false;
      const lower = filename.toLowerCase();
      const parts = lower.split('.');
      if (parts.length <= 1) return false;
      const ext = parts.pop();

      return [
        'exe', 'msi', 'com', 'scr', 'bat', 'cmd',
        'ps1', 'vbs', 'sh',
        'jar',
        'apk', 'aab',
        'appimage', 'deb', 'rpm'
      ].includes(ext);
    }

    function getFileIconSVG(filename) {
      const cat = getFileCategory(filename);
      return PIXEL_ICONS[cat] || PIXEL_ICONS.generic;
    }

    function showInactive(title, message) {
      isTerminated = true;
      if (pollTimer) clearTimeout(pollTimer);
      if (activeCard) {
        activeCard.classList.add('hidden');
        activeCard.style.display = 'none';
      }
      if (pinCard) {
        pinCard.classList.add('hidden');
        pinCard.style.display = 'none';
      }
      const closingCard = document.getElementById('room-closing-card');
      if (closingCard) {
        closingCard.style.display = 'none';
      }
      const inactiveEl = document.getElementById('room-inactive-card') || document.getElementById('card-inactive') || inactiveCard;
      if (inactiveEl) {
        const titleEl = document.getElementById('inactive-title') || inactiveEl.querySelector('h2');
        const msgEl = document.getElementById('inactive-message') || inactiveEl.querySelector('p');
        if (titleEl && title) titleEl.textContent = title;
        if (msgEl && message) msgEl.textContent = message;
        inactiveEl.classList.remove('hidden');
        inactiveEl.style.display = 'flex';
      }
    }

    let closingTimer = null;
    function startClosingCountdown(remainingSec) {
      const activeCard = document.getElementById('room-active-card');
      const closingCard = document.getElementById('room-closing-card');
      const pinCard = document.getElementById('pin-barrier-card') || document.getElementById('room-pin-card');
      const sidebar = document.querySelector('.sidebar');
      const mainViewport = document.querySelector('.main-viewport');

      if (activeCard) {
        activeCard.classList.add('hidden');
        activeCard.style.display = 'none';
      }
      if (pinCard) {
        pinCard.classList.add('hidden');
        pinCard.style.display = 'none';
      }
      if (sidebar) {
        sidebar.style.display = 'none';
      }
      if (mainViewport) {
        mainViewport.style.padding = '0';
        mainViewport.style.maxWidth = '100%';
        mainViewport.style.margin = '0';
        mainViewport.style.width = '100%';
      }
      if (closingCard) {
        closingCard.classList.remove('hidden');
        closingCard.style.display = 'flex';
      }

      let currentSec = typeof remainingSec === 'number' ? Math.max(0, Math.floor(remainingSec)) : 10;
      const totalSec = 10;
      const circumference = 2 * Math.PI * 72; // ~452.389

      const ringFill = document.getElementById('closing-ring-fill');
      const countdownNum = document.getElementById('closing-countdown');

      function updateRing(sec) {
        if (countdownNum) {
          countdownNum.textContent = String(sec);
        }
        if (ringFill) {
          const ratio = Math.max(0, Math.min(1, (totalSec - sec) / totalSec));
          const offset = circumference * ratio;
          ringFill.style.strokeDashoffset = String(offset);
        }
      }

      updateRing(currentSec);

      if (closingTimer) clearInterval(closingTimer);
      closingTimer = setInterval(() => {
        currentSec--;
        if (currentSec <= 0) {
          clearInterval(closingTimer);
          updateRing(0);
          showInactive('Room Closed', 'This temporary transfer room has been closed.');
        } else {
          updateRing(currentSec);
        }
      }, 1000);
    }

    const cancelCloseBtn = document.getElementById('cancel-closing-btn');
    if (cancelCloseBtn) {
      cancelCloseBtn.addEventListener('click', () => {
        window.location.reload();
      });
    }

    const initialStatus = document.body.dataset.status;
    const initialClosingSec = parseInt(document.body.dataset.closingSeconds || '10', 10);
    if (initialStatus === 'closing') {
      startClosingCountdown(initialClosingSec);
    }

    // Countdown loop
    function updateCountdown() {
      if (isTerminated) return;
      const now = Date.now();
      const remainingMs = expiresAt - now;
      const remainingSec = Math.max(0, Math.floor(remainingMs / 1000));

      if (countdownEl) {
        countdownEl.textContent = formatTime(remainingSec);
      }

      if (remainingSec <= 0 && expiresAt > 0) {
        showInactive('Room Expired', 'This temporary room has reached its lifespan and is no longer accessible.');
        return;
      }
      setTimeout(updateCountdown, 1000);
    }
    updateCountdown();

    // --------------------------------------------------------------------------
    // Recent Activity Feed
    // --------------------------------------------------------------------------
    function addRecentActivity(type, filename, subtext) {
      recentActivities.unshift({
        type, // "upload" or "download"
        filename,
        subtext,
        time: 'Just now',
        timestamp: Date.now(),
      });

      if (recentActivities.length > 8) {
        recentActivities.pop();
      }
      renderActivityList();
    }

    function renderActivityList() {
      const activityList = document.getElementById('activity-list');
      if (!activityList) return;

      if (recentActivities.length === 0) {
        activityList.innerHTML = `
          <div class="panel-empty-state">
            <div class="empty-icon-circle">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>
            </div>
            <div class="empty-title">No transfer activity yet</div>
            <div class="empty-sub">Files and text you share will appear here</div>
          </div>
        `;
        return;
      }

      activityList.innerHTML = '';
      recentActivities.forEach((act) => {
        const row = document.createElement('div');
        row.className = 'activity-row';

        const mainDiv = document.createElement('div');
        mainDiv.className = 'activity-main';

        const iconDiv = document.createElement('div');
        iconDiv.className = `activity-direction-icon ${act.type}`;
        if (act.type === 'download') {
          iconDiv.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="5" x2="12" y2="19"></line><polyline points="19 12 12 19 5 12"></polyline></svg>`;
        } else {
          iconDiv.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="19" x2="12" y2="5"></line><polyline points="5 12 12 5 19 12"></polyline></svg>`;
        }

        const metaDiv = document.createElement('div');
        metaDiv.className = 'activity-meta';

        const nameSpan = document.createElement('span');
        nameSpan.className = 'activity-filename';
        nameSpan.textContent = act.filename;

        const subSpan = document.createElement('span');
        subSpan.className = 'activity-subtext';
        subSpan.textContent = act.subtext;

        metaDiv.appendChild(nameSpan);
        metaDiv.appendChild(subSpan);
        mainDiv.appendChild(iconDiv);
        mainDiv.appendChild(metaDiv);

        const timeSpan = document.createElement('span');
        timeSpan.className = 'activity-time';
        timeSpan.textContent = act.time;

        row.appendChild(mainDiv);
        row.appendChild(timeSpan);
        activityList.appendChild(row);
      });
    }

    const clearActBtn = document.getElementById('btn-clear-activity');
    if (clearActBtn) {
      clearActBtn.addEventListener('click', () => {
        recentActivities.length = 0;
        renderActivityList();
      });
    }

    // --------------------------------------------------------------------------
    // Connected Participants (Authoritative Backend State)
    // --------------------------------------------------------------------------
    function getDeviceIconSVG(name) {
      const n = (name || '').toLowerCase();
      if (n.includes('win') || (!n && page === 'creator')) {
        return `<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M0 3.449L9.75 2.1v9.451H0m10.949-9.602L24 0v11.4H10.949M0 12.6h9.75v9.451L0 20.699M10.949 12.6H24V24l-12.951-1.898"/></svg>`;
      }
      if (n.includes('mac') || n.includes('iphone') || n.includes('ipad') || n.includes('apple')) {
        return `<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M15.97 6.84c.62-.75 1.04-1.8 0.92-2.84-.9.04-2 0.6-2.65 1.34-.56.63-1.05 1.68-.92 2.7 1.01.08 2.03-.45 2.65-1.2"/></svg>`;
      }
      if (n.includes('android')) {
        return `<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M17.523 15.3414c-.5511 0-.9993-.4486-.9993-.9997s.4482-.9993.9993-.9993c.551 0 .9993.4482.9993.9993.0001.5511-.4483.9997-.9993.9997m-11.046 0c-.5511 0-.9993-.4486-.9993-.9997s.4482-.9993.9993-.9993c.5511 0 .9993.4482.9993.9993 0 .5511-.4482.9997-.9993.9997m11.4045-6.02l1.9973-3.4592a.416.416 0 00-.1521-.5676.416.416 0 00-.5676.1521l-2.0223 3.503C15.5902 8.4116 13.8533 8.0833 12 8.0833s-3.5902.3283-5.1367.8664L4.841 5.4467a.4161.4161 0 00-.5677-.1521.4157.4157 0 00-.1521.5676l1.9973 3.4592C2.6889 11.1867.3432 14.6589 0 18.761h24c-.3432-4.1021-2.6889-7.5743-6.1185-9.4396"/></svg>`;
      }
      return `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2" ry="2"></rect><line x1="12" y1="18" x2="12.01" y2="18"></line></svg>`;
    }

    function renderParticipantList(participants, authoritativeCount) {
      const list = document.getElementById('participant-list');
      const countBadge = document.getElementById('participant-count');
      const metricCount = document.getElementById('metric-connected-count');
      if (!list) return;

      const remoteCount = participants ? participants.length : 0;
      const totalCount = page === 'creator' ? Math.max(1, remoteCount + 1) : (typeof authoritativeCount === 'number' ? authoritativeCount : remoteCount);
      if (countBadge) countBadge.textContent = String(totalCount);
      if (metricCount) metricCount.textContent = String(totalCount);

      list.innerHTML = '';

      // If creator page, always show Host ("You")
      if (page === 'creator') {
        const hostRow = document.createElement('div');
        hostRow.className = 'participant-row';
        hostRow.innerHTML = `
          <div class="participant-info">
            <div class="participant-icon">
              ${getDeviceIconSVG(navigator.userAgent || 'Windows')}
            </div>
            <div class="participant-details">
              <div class="participant-name-row">
                <span class="participant-name">Windows Device</span>
                <span class="participant-badge-you">You</span>
              </div>
              <span class="participant-ip">${window.location.hostname || '192.168.68.55'}</span>
            </div>
          </div>
          <div class="participant-status-wrap">
            <span class="participant-status-active">
              <span class="status-indicator-dot green"></span> Active
            </span>
            <button type="button" class="participant-options-btn" title="Options" aria-label="Device options">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="2"></circle><circle cx="19" cy="12" r="2"></circle><circle cx="5" cy="12" r="2"></circle></svg>
            </button>
          </div>
        `;
        list.appendChild(hostRow);
      }

      if (participants && participants.length > 0) {
        participants.forEach((p) => {
          const row = document.createElement('div');
          row.className = 'participant-row';

          const infoDiv = document.createElement('div');
          infoDiv.className = 'participant-info';

          const iconDiv = document.createElement('div');
          iconDiv.className = 'participant-icon';
          iconDiv.innerHTML = getDeviceIconSVG(p.name || '');

          const detailsDiv = document.createElement('div');
          detailsDiv.className = 'participant-details';

          const nameRow = document.createElement('div');
          nameRow.className = 'participant-name-row';

          const nameSpan = document.createElement('span');
          nameSpan.className = 'participant-name';
          nameSpan.textContent = p.name || 'Mobile Device';
          nameRow.appendChild(nameSpan);

          const ipSpan = document.createElement('span');
          ipSpan.className = 'participant-ip font-mono';
          ipSpan.textContent = p.ip || 'LAN Peer';

          detailsDiv.appendChild(nameRow);
          detailsDiv.appendChild(ipSpan);
          infoDiv.appendChild(iconDiv);
          infoDiv.appendChild(detailsDiv);

          const statusWrap = document.createElement('div');
          statusWrap.className = 'participant-status-wrap';
          statusWrap.innerHTML = `
            <span class="participant-status-active">
              <span class="status-indicator-dot green"></span> Active
            </span>
            <button type="button" class="participant-options-btn" title="Options" aria-label="Device options">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="2"></circle><circle cx="19" cy="12" r="2"></circle><circle cx="5" cy="12" r="2"></circle></svg>
            </button>
          `;

          row.appendChild(infoDiv);
          row.appendChild(statusWrap);
          list.appendChild(row);
        });
      }
    }

    const copyAllPartBtn = document.getElementById('btn-copy-all-participants');
    if (copyAllPartBtn) {
      copyAllPartBtn.addEventListener('click', async () => {
        const list = document.getElementById('participant-list');
        if (!list) return;
        const text = list.innerText || '';
        await copyTextToClipboard(text, copyAllPartBtn);
      });
    }

    // --------------------------------------------------------------------------
    // File List Rendering (Island 3)
    // --------------------------------------------------------------------------
    function renderFileList(files) {
      const fileListEl = document.getElementById('file-list');
      const fileCountEl = document.getElementById('file-count');
      const navCountEl = document.getElementById('nav-file-count');
      const metricTransferred = document.getElementById('metric-transferred-size');
      if (!fileListEl) return;

      const count = files ? files.length : 0;
      if (fileCountEl) fileCountEl.textContent = String(count);
      if (navCountEl) navCountEl.textContent = String(count);

      let totalBytes = 0;
      if (files) {
        files.forEach((f) => {
          totalBytes += (f.size_bytes || 0);
        });
      }
      if (metricTransferred) {
        metricTransferred.textContent = formatBytes(totalBytes);
      }

      if (count === 0) {
        fileListEl.innerHTML = `
          <div class="panel-empty-state large-empty">
            <div class="empty-folder-icon">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#FF7800" stroke-width="1.6"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
            </div>
            <div class="empty-title large-title">Nothing here yet.</div>
            <div class="empty-sub">Waiting for files. Drag files from your device or use the upload area.</div>
          </div>
        `;
        return;
      }

      fileListEl.innerHTML = '';
      const orderedFiles = [...files].reverse();
      orderedFiles.forEach((file) => {
        const row = document.createElement('div');
        row.className = 'file-row';
        row.dataset.fileId = file.file_id;

        const mainDiv = document.createElement('div');
        mainDiv.className = 'file-main';

        const iconDiv = document.createElement('div');
        iconDiv.className = 'file-icon-box';
        iconDiv.innerHTML = getFileIconSVG(file.filename);

        const textCol = document.createElement('div');
        textCol.className = 'file-text-col';

        const nameSpan = document.createElement('span');
        nameSpan.className = 'file-title';
        nameSpan.title = file.filename;
        nameSpan.textContent = file.filename;

        const cat = getFileCategory(file.filename).toUpperCase();
        const specsSpan = document.createElement('span');
        specsSpan.className = 'file-specs font-mono';
        specsSpan.textContent = `${formatBytes(file.size_bytes)} · ${cat}`;

        textCol.appendChild(nameSpan);
        textCol.appendChild(specsSpan);
        mainDiv.appendChild(iconDiv);
        mainDiv.appendChild(textCol);

        const actionsDiv = document.createElement('div');
        actionsDiv.className = 'file-actions';

        if (isPotentiallyExecutable(file.filename)) {
          const warnSpan = document.createElement('span');
          warnSpan.className = 'badge-exec-warning';
          warnSpan.tabIndex = 0;
          warnSpan.setAttribute('role', 'tooltip');
          const warnText = t.executableWarning || 'Potentially executable file. Only open or install files you trust.';
          warnSpan.setAttribute('aria-label', warnText);
          warnSpan.title = warnText;
          warnSpan.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
            <span class="warning-tooltip">${warnText}</span>
          `;
          actionsDiv.appendChild(warnSpan);
        }

        if (globalShareEnabled && page === 'creator') {
          const shareBtn = document.createElement('button');
          shareBtn.type = 'button';
          shareBtn.className = 'btn btn-secondary btn-sm btn-share-link';
          shareBtn.dataset.fileId = file.file_id;
          shareBtn.dataset.fileName = file.filename;
          shareBtn.textContent = 'Share Link';
          shareBtn.addEventListener('click', () => openShareModal(file.file_id, file.filename));
          actionsDiv.appendChild(shareBtn);
        }

        const isDownloaded = downloadedFiles.has(file.file_id);
        const downloadLink = document.createElement('a');
        downloadLink.href = `/api/v1/rooms/${encodeURIComponent(token)}/files/${encodeURIComponent(file.file_id)}`;
        downloadLink.download = file.filename;

        if (isDownloaded) {
          downloadLink.className = 'btn btn-sm btn-saved';
          downloadLink.textContent = '✓ Saved';
        } else {
          downloadLink.className = 'btn btn-sm btn-download';
          downloadLink.innerHTML = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg> Download`;
          downloadLink.addEventListener('click', () => {
            markFileDownloaded(file.file_id, file.filename);
            downloadLink.className = 'btn btn-sm btn-saved';
            downloadLink.textContent = '✓ Saved';
          });
        }
        actionsDiv.appendChild(downloadLink);

        row.appendChild(mainDiv);
        row.appendChild(actionsDiv);
        fileListEl.appendChild(row);
      });
    }

    // --------------------------------------------------------------------------
    // Client Ephemeral Session ID (Per Room)
    // --------------------------------------------------------------------------
    function getClientSessionId() {
      const key = `hamal_client_session_${token}`;
      let sid = '';
      try {
        sid = sessionStorage.getItem(key) || '';
      } catch (_) {}
      if (!sid) {
        if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
          sid = 'cs_' + crypto.randomUUID().replace(/-/g, '');
        } else if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
          const arr = new Uint8Array(16);
          crypto.getRandomValues(arr);
          sid = 'cs_' + Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
        } else {
          sid = 'cs_' + Date.now().toString(36);
        }
        try {
          sessionStorage.setItem(key, sid);
        } catch (_) {}
      }
      return sid;
    }

    async function copyTextToClipboard(text, btn) {
      if (!text) return;
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(text);
        } else {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }
      } catch (_) {
        try {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        } catch (_) {}
      }
      if (btn) {
        btn.classList.add('copied');
        const copyLabel = btn.querySelector('.copy-label');
        if (copyLabel) copyLabel.textContent = t.copiedText || 'Copied!';
        setTimeout(() => {
          btn.classList.remove('copied');
          if (copyLabel) copyLabel.textContent = t.copyText || 'Copy';
        }, 2000);
      }
    }

    // --------------------------------------------------------------------------
    // Text List Rendering (Direct Text Sharing Stream)
    // --------------------------------------------------------------------------
    function renderTextList(texts) {
      const textListEl = document.getElementById('text-list');
      const textCountEl = document.getElementById('text-count');
      const tabBadgeEl = document.getElementById('tab-text-badge');
      const navCountEl = document.getElementById('nav-text-count');

      if (!textListEl) return;

      const count = texts ? texts.length : 0;
      if (textCountEl) textCountEl.textContent = String(count);
      if (tabBadgeEl) tabBadgeEl.textContent = String(count);
      if (navCountEl) navCountEl.textContent = String(count);

      if (count === 0) {
        textListEl.innerHTML = `
          <div class="panel-empty-state">
            <div class="empty-title" id="no-texts-title">${t.noTextsYet || 'No text messages yet.'}</div>
            <div class="empty-sub" id="no-texts-desc">${t.noTextsSub || 'Paste commands, logs, snippets, or URLs to share directly.'}</div>
          </div>
        `;
        return;
      }

      const wasNearBottom = (textListEl.scrollHeight - textListEl.scrollTop - textListEl.clientHeight) < 70;
      textListEl.innerHTML = '';

      texts.forEach((item) => {
        const isServer = item.sender_type === 'server';
        const isSelf = Boolean(item.is_self);
        const card = document.createElement('div');
        card.className = `text-item-card sender-${isServer ? 'server' : 'client'} ${isServer ? 'sender-server' : (isSelf ? 'client-self' : 'client-peer')}`;
        card.dataset.textId = item.id;

        const header = document.createElement('div');
        header.className = 'text-item-header';

        const metaDiv = document.createElement('div');
        metaDiv.className = 'text-sender-meta';

        const senderTag = document.createElement('div');
        senderTag.className = `text-sender-tag ${isServer ? 'server' : (isSelf ? 'client-self' : 'client-peer')}`;

        const roleSpan = document.createElement('span');
        roleSpan.className = 'sender-role font-mono';
        roleSpan.textContent = isServer ? (t.server || 'SERVER') : (t.client || 'CLIENT');
        senderTag.appendChild(roleSpan);
        metaDiv.appendChild(senderTag);

        if (item.created_at) {
          const timeSpan = document.createElement('span');
          timeSpan.className = 'text-item-time';
          try {
            const d = new Date(item.created_at);
            timeSpan.textContent = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
          } catch (_) {}
          metaDiv.appendChild(timeSpan);
        }
        header.appendChild(metaDiv);

        if (!isServer) {
          const copyBtn = document.createElement('button');
          copyBtn.type = 'button';
          copyBtn.className = 'btn-copy-text';
          copyBtn.title = t.copyText || 'Copy';
          copyBtn.setAttribute('aria-label', 'Copy message text');
          copyBtn.innerHTML = `
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>
            <span class="copy-label">${t.copyText || 'Copy'}</span>
          `;

          copyBtn.addEventListener('click', async () => {
            await copyTextToClipboard(item.content, copyBtn);
          });
          header.appendChild(copyBtn);
        }

        card.appendChild(header);

        const contentDiv = document.createElement('div');
        contentDiv.className = 'text-item-content font-mono';

        const urlRegex = /(https?:\/\/[^\s<>"'`]+)/g;
        let lastIdx = 0;
        let match;
        const contentText = item.content || '';

        while ((match = urlRegex.exec(contentText)) !== null) {
          if (match.index > lastIdx) {
            contentDiv.appendChild(document.createTextNode(contentText.substring(lastIdx, match.index)));
          }
          const url = match[0];
          const link = document.createElement('a');
          link.href = url;
          link.target = '_blank';
          link.rel = 'noopener noreferrer';
          link.className = 'text-content-link';
          link.textContent = url;
          contentDiv.appendChild(link);
          lastIdx = match.index + url.length;
        }
        if (lastIdx < contentText.length) {
          contentDiv.appendChild(document.createTextNode(contentText.substring(lastIdx)));
        }

        card.appendChild(contentDiv);
        textListEl.appendChild(card);
      });

      if (wasNearBottom) {
        textListEl.scrollTop = textListEl.scrollHeight;
      }
    }

    function bindExistingCopyButtons() {
      document.querySelectorAll('.btn-copy-text').forEach((btn) => {
        if (btn.dataset.bound) return;
        btn.dataset.bound = 'true';
        btn.addEventListener('click', async () => {
          const card = btn.closest('.text-item-card');
          const contentEl = card ? card.querySelector('.text-item-content') : null;
          const content = contentEl ? contentEl.textContent : '';
          await copyTextToClipboard(content, btn);
        });
      });
    }

    // --------------------------------------------------------------------------
    // Polling Loop
    // --------------------------------------------------------------------------
    async function pollStatus() {
      if (isTerminated || !token) return;

      try {
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
            showInactive('Room Closed', 'This temporary room is no longer accessible.');
            return;
          } else if (data.status === 'expired' || data.remaining_seconds <= 0) {
            showInactive('Room Expired', 'This temporary room has expired.');
            return;
          } else if (data.status === 'closing') {
            startClosingCountdown(data.closing_remaining_seconds);
            if (data.closing_remaining_seconds <= 0) {
              showInactive('Room Closed', 'This temporary room is no longer accessible.');
              return;
            }
          }

          // Authoritative participant count & active peer list
          renderParticipantList(data.participants || [], data.participant_count);

          if (page === 'participant' && data.pin_required && !data.pin_authenticated) {
            if (pinCard) {
              pinCard.classList.remove('hidden');
              pinCard.style.display = 'block';
            }
            if (activeCard) {
              activeCard.classList.add('hidden');
              activeCard.style.display = 'none';
            }
          } else if (page === 'participant' && data.status !== 'closing') {
            if (pinCard) {
              pinCard.classList.add('hidden');
              pinCard.style.display = 'none';
            }
            if (activeCard) {
              activeCard.classList.remove('hidden');
              activeCard.style.display = 'block';
            }
          }
        }

        const filesRes = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/files`, {
          cache: 'no-store',
        });
        if (filesRes.status === 404 || filesRes.status === 410) {
          showInactive('Room Closed', 'This temporary room is no longer accessible.');
          return;
        }
        if (filesRes.ok) {
          const filesData = await filesRes.json();
          renderFileList(filesData.files || []);
        }

        const textsRes = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/texts`, {
          cache: 'no-store',
          headers: {
            'X-Client-Session-ID': getClientSessionId(),
          },
        });
        if (textsRes.status === 404 || textsRes.status === 410) {
          showInactive('Room Closed', 'This temporary room is no longer accessible.');
          return;
        }
        if (textsRes.ok) {
          const textsData = await textsRes.json();
          renderTextList(textsData.texts || []);
        }
      } catch (e) {
        // Network glitches are gracefully skipped during polling
      }

      const nextInterval = document.hidden ? 12000 : 3500;
      pollTimer = setTimeout(pollStatus, nextInterval);
    }

    // Start initial polling
    pollTimer = setTimeout(pollStatus, 2000);
    renderActivityList();
    renderParticipantList([], 0);
    bindExistingCopyButtons();

    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && !isTerminated) {
        if (pollTimer) clearTimeout(pollTimer);
        pollStatus();
      }
    });

    // --------------------------------------------------------------------------
    // File Upload Handling (Drag & Drop + Streaming Progress)
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
        if (progressContainer) progressContainer.style.display = 'none';
        return;
      }
      isUploading = true;
      const file = uploadQueue.shift();
      uploadSingleFile(file);
    }

    function uploadSingleFile(file) {
      if (progressContainer) progressContainer.style.display = 'block';
      if (progressFilename) progressFilename.textContent = file.name;
      if (progressPercent) progressPercent.textContent = '0%';
      if (progressFill) progressFill.style.width = '0%';
      if (uploadError) uploadError.style.display = 'none';

      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/v1/rooms/${encodeURIComponent(token)}/files?filename=${encodeURIComponent(file.name)}`);
      xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream');
      xhr.setRequestHeader('X-Client-Session-ID', getClientSessionId());

      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable) {
          const percent = Math.round((e.loaded / e.total) * 100);
          if (progressPercent) progressPercent.textContent = `${percent}%`;
          if (progressFill) progressFill.style.width = `${percent}%`;
        }
      });

      xhr.addEventListener('load', () => {
        if (xhr.status === 201 || xhr.status === 200) {
          try {
            const data = JSON.parse(xhr.responseText);
            addRecentActivity('upload', file.name, formatBytes(file.size));
            if (data.file) {
              // Immediately fetch updated file list
              fetch(`/api/v1/rooms/${encodeURIComponent(token)}/files`, { cache: 'no-store' })
                .then((r) => r.json())
                .then((fData) => renderFileList(fData.files || []));
            }
          } catch (e) {}
          processNextUpload();
      };

      xhr.onerror = () => {
        showUploadError(`${file.name}: Upload failed or connection interrupted.`);
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
        if (dropzoneTitle) dropzoneTitle.textContent = 'UPLOAD FILES';
      });

      dropzone.addEventListener('drop', (e) => {
        e.preventDefault();
        dropzone.classList.remove('dragover');
        if (dropzoneTitle) dropzoneTitle.textContent = 'UPLOAD FILES';
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
    // Copy Room URL
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
            setTimeout(() => copyToast.classList.remove('show'), 2000);
          }
        } catch (err) {
          linkInput.select();
          document.execCommand('copy');
          if (copyToast) {
            copyToast.classList.add('show');
            setTimeout(() => copyToast.classList.remove('show'), 2000);
          }
        }
      });
    }

    // --------------------------------------------------------------------------
    // QR Code Lightbox Modal
    // --------------------------------------------------------------------------
    const qrBox = document.getElementById('qr-box');
    const qrImage = document.getElementById('qr-image');
    if (qrBox && qrImage) {
      qrBox.addEventListener('click', () => {
        const lightboxBox = document.getElementById('qr-lightbox-box');
        if (lightboxBox) {
          const lightboxImage = document.createElement('img');
          lightboxImage.src = qrImage.src;
          lightboxImage.alt = 'Participant QR Code';
          lightboxImage.width = 220;
          lightboxImage.height = 220;
          lightboxImage.style.width = '100%';
          lightboxImage.style.height = '100%';
          lightboxBox.replaceChildren(lightboxImage);
        }
        showModal('modal-qr');
      });
    }
    const modalQrClose = document.getElementById('modal-qr-close');
    if (modalQrClose) {
      modalQrClose.addEventListener('click', closeAllModals);
    }
    
    // --------------------------------------------------------------------------
    // Close Room Modal (Creator Desktop & Participant Mobile)
    // --------------------------------------------------------------------------
    const closeBtn = document.getElementById('close-room-btn');
    if (closeBtn) {
      closeBtn.addEventListener('click', () => {
        showModal('modal-close-confirm');
      });
    }
    const closeCancel = document.getElementById('modal-close-cancel');
    if (closeCancel) {
      closeCancel.addEventListener('click', closeAllModals);
    }
    const confirmCloseBtn = document.getElementById('confirm-close-btn');
    if (confirmCloseBtn) {
      confirmCloseBtn.addEventListener('click', async () => {
        confirmCloseBtn.disabled = true;
        confirmCloseBtn.textContent = 'Closing room…';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/close`, {
            method: 'POST',
          });
          closeAllModals();
          if (res.ok) {
            const data = await res.json().catch(() => ({}));
            if (data.status === 'closing') {
              startClosingCountdown(data.closing_remaining_seconds);
            } else {
              showInactive('Room Closed', 'This temporary transfer room has been closed and all files purged.');
            }
          } else if (res.status === 404 || res.status === 410) {
            showInactive('Room Closed', 'This temporary transfer room has been closed and all files purged.');
          } else {
            const errData = await res.json().catch(() => ({}));
            alert(errData.error || 'Failed to close room');
            confirmCloseBtn.disabled = false;
            confirmCloseBtn.textContent = 'Yes, Close Room';
          }
        } catch (e) {
          closeAllModals();
          showInactive('Room Closed', 'This temporary transfer room has been closed and all files purged.');
        }
      });
    }

    // Participant Mobile Close Room Modal & Confirmation
    const participantCloseBtn = document.getElementById('participant-close-btn');
    const closeConfirmModal = document.getElementById('close-confirm-modal');
    const participantModalClose = document.getElementById('participant-modal-close');
    const participantCancelClose = document.getElementById('participant-cancel-close-btn');
    const participantConfirmClose = document.getElementById('participant-confirm-close-btn');

    if (participantCloseBtn && closeConfirmModal) {
      participantCloseBtn.addEventListener('click', () => {
        closeConfirmModal.style.display = 'flex';
      });

      if (participantModalClose) {
        participantModalClose.addEventListener('click', () => {
          closeConfirmModal.style.display = 'none';
        });
      }

      if (participantCancelClose) {
        participantCancelClose.addEventListener('click', () => {
          closeConfirmModal.style.display = 'none';
        });
      }

      closeConfirmModal.addEventListener('click', (e) => {
        if (e.target === closeConfirmModal) {
          closeConfirmModal.style.display = 'none';
        }
      });

      if (participantConfirmClose) {
        participantConfirmClose.addEventListener('click', async () => {
          participantConfirmClose.disabled = true;
          participantConfirmClose.textContent = t.closing || 'Closing room…';

          try {
            const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/close`, {
              method: 'POST',
            });
            closeConfirmModal.style.display = 'none';

            if (res.ok) {
              const data = await res.json().catch(() => ({}));
              if (data.status === 'closing') {
                startClosingCountdown(data.closing_remaining_seconds);
              } else {
                showInactive('Room Closed', 'This temporary room is no longer accessible.');
              }
            } else if (res.status === 404 || res.status === 410) {
              showInactive('Room Closed', 'This temporary room is no longer accessible.');
            } else {
              const errData = await res.json().catch(() => ({}));
              alert(errData.error || t.closeError || 'Failed to close room');
              participantConfirmClose.disabled = false;
              participantConfirmClose.textContent = t.confirmBtn || 'Yes, Close Room';
            }
          } catch (e) {
            closeConfirmModal.style.display = 'none';
            showInactive('Room Closed', 'This temporary room is no longer accessible.');
          }
        });
      }
    }

    // --------------------------------------------------------------------------
    // Global Share Modal (Creator View)
    // --------------------------------------------------------------------------
    function openShareModal(fileId, filename) {
      const modalFileId = document.getElementById('modal-file-id');
      const subtitle = document.getElementById('modal-file-subtitle');
      const resultBox = document.getElementById('share-result-box');
      const errorBox = document.getElementById('share-error-box');

      if (modalFileId) modalFileId.value = fileId;
      if (subtitle) subtitle.textContent = `Generate a public download link for "${filename}".`;
      if (resultBox) resultBox.style.display = 'none';
      if (errorBox) errorBox.style.display = 'none';

      showModal('share-modal');
    }

    const shareCloseBtn = document.getElementById('close-modal-btn');
    if (shareCloseBtn) {
      shareCloseBtn.addEventListener('click', closeAllModals);
    }

    const shareForm = document.getElementById('create-share-form');
    if (shareForm) {
      shareForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const fileId = document.getElementById('modal-file-id').value;
        const ttlSelect = document.getElementById('share-ttl-select');
        const ttlSeconds = parseInt(ttlSelect ? ttlSelect.value : '3600', 10);
        const generateBtn = document.getElementById('generate-share-btn');
        const resultBox = document.getElementById('share-result-box');
        const shareInput = document.getElementById('generated-share-input');
        const errorBox = document.getElementById('share-error-box');

        if (generateBtn) generateBtn.disabled = true;
        if (errorBox) errorBox.style.display = 'none';

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/files/${encodeURIComponent(fileId)}/share`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ttl_seconds: ttlSeconds }),
          });

          if (res.ok) {
            const data = await res.json();
            if (shareInput) shareInput.value = data.share_url;
            if (resultBox) resultBox.style.display = 'block';
          } else {
            const errData = await res.json().catch(() => ({}));
            if (errorBox) {
              errorBox.textContent = errData.error || 'Failed to create share';
              errorBox.style.display = 'block';
            }
          }
        } catch (err) {
          if (errorBox) {
            errorBox.textContent = 'Network error while creating share';
            errorBox.style.display = 'block';
          }
        } finally {
          if (generateBtn) generateBtn.disabled = false;
        }
      });
    }

    const copyShareBtn = document.getElementById('copy-share-btn');
    if (copyShareBtn) {
      copyShareBtn.addEventListener('click', async () => {
        const shareInput = document.getElementById('generated-share-input');
        const toast = document.getElementById('copy-share-toast');
        if (shareInput && shareInput.value) {
          try {
            await navigator.clipboard.writeText(shareInput.value);
            if (toast) {
              toast.classList.add('show');
              setTimeout(() => toast.classList.remove('show'), 2000);
            }
          } catch (e) {
            shareInput.select();
            document.execCommand('copy');
            if (toast) {
              toast.classList.add('show');
              setTimeout(() => toast.classList.remove('show'), 2000);
            }
          }
        }
      });
    }

    // --------------------------------------------------------------------------
    // Participant PIN Authentication Form & 8-Slot Coordinator
    // --------------------------------------------------------------------------
    const pinForm = document.getElementById('pin-form');
    const pinInput = document.getElementById('participant-pin-input');
    const pinSlotsWrapper = document.getElementById('pin-slots-wrapper') || document.querySelector('.pin-slots-wrapper');
    const pinSlots = document.querySelectorAll('.pin-slot');
    const unlockBtn = document.getElementById('unlock-btn');
    const unlockBtnText = document.getElementById('unlock-btn-text');
    const pinError = document.getElementById('pin-error');
    const pinErrorText = document.getElementById('pin-error-text');
    const pinCooldown = document.getElementById('pin-cooldown');
    const pinCooldownText = document.getElementById('pin-cooldown-text');

    function syncPinSlots() {
      if (!pinInput || !pinSlots.length) return;
      const val = pinInput.value;
      const len = val.length;
      const isFocused = document.activeElement === pinInput || document.activeElement === pinSlotsWrapper;

      pinSlots.forEach((slot, idx) => {
        slot.classList.remove('active', 'filled');
        if (idx < len) {
          slot.classList.add('filled');
          slot.innerHTML = `<span class="pin-dot-filled">•</span>`;
        } else if (idx === len && isFocused) {
          slot.classList.add('active');
          slot.innerHTML = `<span class="pin-caret"></span>`;
        } else {
          slot.innerHTML = `<span class="pin-dot">•</span>`;
        }
      });
    }

    if (pinSlotsWrapper && pinInput) {
      pinSlotsWrapper.addEventListener('click', () => {
        pinInput.focus();
      });
      pinInput.addEventListener('input', () => {
        syncPinSlots();
      });
      pinInput.addEventListener('focus', syncPinSlots);
      pinInput.addEventListener('blur', syncPinSlots);
      pinInput.addEventListener('keyup', (e) => {
        syncPinSlots();
      });

      syncPinSlots();
      if (pinCard && pinCard.style.display !== 'none') {
        setTimeout(() => {
          pinInput.focus();
          syncPinSlots();
        }, 150);
      }
    }

    if (pinForm) {
      pinForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const pinVal = pinInput ? pinInput.value.trim() : '';
        if (!pinVal) return;

        if (unlockBtn) {
          unlockBtn.disabled = true;
          if (unlockBtnText) {
            unlockBtnText.textContent = 'Verifying…';
          } else {
            unlockBtn.textContent = 'Verifying…';
          }
        }
        if (pinError) {
          pinError.style.display = 'none';
          if (pinErrorText) pinErrorText.textContent = '';
        }

        try {
          const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/auth/pin`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: JSON.stringify({ pin: pinVal }),
          });

          if (res.ok) {
            // PIN authenticated successfully!
            if (pinCard) {
              pinCard.style.display = 'none';
              pinCard.classList.add('hidden');
            }
            if (activeCard) {
              activeCard.style.display = 'flex';
              activeCard.classList.remove('hidden');
            }
            // Trigger immediate room and files polling
            if (pollTimer) clearTimeout(pollTimer);
            pollStatus();
          } else {
            const data = await res.json().catch(() => ({}));
            if (res.status === 429) {
              const retryAfter = data.retry_after_seconds || 30;
              if (pinCooldown) {
                pinCooldown.style.display = 'block';
                if (pinCooldownText) {
                  pinCooldownText.textContent = `Too many failed attempts. Cooldown active (${retryAfter}s).`;
                }
              }
              if (pinInput) pinInput.disabled = true;
              if (unlockBtn) {
                unlockBtn.disabled = true;
                if (unlockBtnText) unlockBtnText.textContent = 'Locked Out';
              }
              let rem = retryAfter;
              const cdTimer = setInterval(() => {
                rem--;
                if (pinCooldownText) {
                  pinCooldownText.textContent = `Too many failed attempts. Cooldown active (${rem}s).`;
                }
                if (rem <= 0) {
                  clearInterval(cdTimer);
                  if (pinCooldown) pinCooldown.style.display = 'none';
                  if (pinInput) {
                    pinInput.disabled = false;
                    pinInput.value = '';
                    syncPinSlots();
                    pinInput.focus();
                  }
                  if (unlockBtn) {
                    unlockBtn.disabled = false;
                    if (unlockBtnText) unlockBtnText.textContent = 'Join Room';
                  }
                }
              }, 1000);
            } else {
              if (pinError) {
                let msg = data.error || 'Incorrect PIN. Please try again.';
                if (data.remaining_attempts !== undefined) {
                  msg += ` (${data.remaining_attempts} attempts remaining)`;
                }
                if (pinErrorText) {
                  pinErrorText.textContent = msg;
                } else {
                  pinError.textContent = msg;
                }
                pinError.style.display = 'flex';
              }
              if (unlockBtn) {
                unlockBtn.disabled = false;
                if (unlockBtnText) unlockBtnText.textContent = 'Join Room';
              }
              if (pinInput) {
                pinInput.value = '';
                syncPinSlots();
                pinInput.focus();
              }
            }
          }
        } catch (err) {
          if (pinError) {
            const errMsg = 'Network error. Please try again.';
            if (pinErrorText) {
              pinErrorText.textContent = errMsg;
            } else {
              pinError.textContent = errMsg;
            }
            pinError.style.display = 'flex';
          }
          if (unlockBtn) {
            unlockBtn.disabled = false;
            if (unlockBtnText) unlockBtnText.textContent = 'Join Room';
          }
        }
      });
    }

    // Navigation tab switching in Creator view
    const navBtnFiles = document.getElementById('nav-btn-files');
    const navBtnRoom = document.getElementById('nav-btn-room');
    const navBtnTexts = document.getElementById('nav-btn-texts');
    if (navBtnFiles) {
      navBtnFiles.addEventListener('click', () => {
        if (navBtnRoom) navBtnRoom.classList.remove('active');
        if (navBtnTexts) navBtnTexts.classList.remove('active');
        navBtnFiles.classList.add('active');
        const fileListEl = document.getElementById('file-list');
        if (fileListEl) {
          fileListEl.scrollIntoView({ behavior: 'smooth' });
        }
      });
    }
    if (navBtnRoom) {
      navBtnRoom.addEventListener('click', () => {
        if (navBtnFiles) navBtnFiles.classList.remove('active');
        if (navBtnTexts) navBtnTexts.classList.remove('active');
        navBtnRoom.classList.add('active');
      });
    }

    // --------------------------------------------------------------------------
    // Mode Switcher (Files vs Text) & Text Sharing Logic
    // --------------------------------------------------------------------------
    const tabFiles = document.getElementById('tab-files');
    const tabText = document.getElementById('tab-text');
    const filesModeContainer = document.getElementById('files-mode-container');
    const textModeContainer = document.getElementById('text-mode-container');
    const creatorDropzone = document.getElementById('dropzone');
    const creatorTextCompose = document.getElementById('creator-text-compose');
    const creatorFilesSection = document.getElementById('creator-files-section');
    const creatorTextsSection = document.getElementById('creator-texts-section');

    function activateFilesTab() {
      if (tabFiles) {
        tabFiles.classList.add('active');
        tabFiles.setAttribute('aria-selected', 'true');
      }
      if (tabText) {
        tabText.classList.remove('active');
        tabText.setAttribute('aria-selected', 'false');
      }
      if (filesModeContainer) filesModeContainer.style.display = '';
      if (textModeContainer) textModeContainer.style.display = 'none';

      if (creatorDropzone) creatorDropzone.style.display = '';
      if (creatorTextCompose) creatorTextCompose.style.display = 'none';
      if (creatorFilesSection) creatorFilesSection.style.display = '';
      if (creatorTextsSection) creatorTextsSection.style.display = 'none';

      if (navBtnFiles) navBtnFiles.classList.add('active');
      if (navBtnTexts) navBtnTexts.classList.remove('active');
    }

    function activateTextTab() {
      if (tabText) {
        tabText.classList.add('active');
        tabText.setAttribute('aria-selected', 'true');
      }
      if (tabFiles) {
        tabFiles.classList.remove('active');
        tabFiles.setAttribute('aria-selected', 'false');
      }
      if (filesModeContainer) filesModeContainer.style.display = 'none';
      if (textModeContainer) textModeContainer.style.display = '';

      if (creatorDropzone) creatorDropzone.style.display = 'none';
      if (creatorTextCompose) creatorTextCompose.style.display = '';
      if (creatorFilesSection) creatorFilesSection.style.display = 'none';
      if (creatorTextsSection) creatorTextsSection.style.display = '';

      if (navBtnTexts) navBtnTexts.classList.add('active');
      if (navBtnFiles) navBtnFiles.classList.remove('active');
    }

    if (tabFiles) {
      tabFiles.addEventListener('click', activateFilesTab);
    }
    if (tabText) {
      tabText.addEventListener('click', activateTextTab);
    }
    if (navBtnTexts) {
      navBtnTexts.addEventListener('click', () => {
        activateTextTab();
        if (creatorTextsSection) {
          creatorTextsSection.scrollIntoView({ behavior: 'smooth' });
        }
      });
    }

    try {
      const urlParams = new URLSearchParams(window.location.search);
      if (window.location.hash === '#text' || urlParams.get('tab') === 'text') {
        activateTextTab();
      }
    } catch (_) {}

    const textInput = document.getElementById('text-input');
    const sendTextBtn = document.getElementById('send-text-btn');
    const textSizeCounter = document.getElementById('text-size-counter');
    const textComposeError = document.getElementById('text-compose-error');
    const maxTextSize = parseInt(document.body.dataset.maxTextSize || '65536', 10);

    function updateTextCounter() {
      if (!textInput || !textSizeCounter) return;
      const val = textInput.value;
      const bytes = new Blob([val]).size;
      const formattedCur = formatBytes(bytes);
      const formattedMax = formatBytes(maxTextSize);
      textSizeCounter.textContent = `${formattedCur} / ${formattedMax}`;
      if (bytes > maxTextSize) {
        textSizeCounter.style.color = 'var(--accent-danger)';
        if (sendTextBtn) sendTextBtn.disabled = true;
      } else {
        textSizeCounter.style.color = '';
        if (sendTextBtn && !sendTextBtn.dataset.sending) sendTextBtn.disabled = false;
      }
    }

    if (textInput) {
      updateTextCounter();
      textInput.addEventListener('input', () => {
        updateTextCounter();
        if (textComposeError) textComposeError.style.display = 'none';
      });

      const isMobile = ('ontouchstart' in window) || (navigator.maxTouchPoints > 0) || window.innerWidth <= 768;
      textInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          if (e.ctrlKey || e.metaKey || (!e.shiftKey && !isMobile)) {
            e.preventDefault();
            submitText();
          }
        }
      });
    }

    async function submitText() {
      if (!textInput || isTerminated || !token) return;
      const content = textInput.value.trim();
      if (!content) return;

      const bytes = new Blob([content]).size;
      if (bytes > maxTextSize) {
        if (textComposeError) {
          textComposeError.textContent = t.textTooLarge || 'Text exceeds maximum allowed size';
          textComposeError.style.display = 'block';
        }
        return;
      }

      if (sendTextBtn) {
        sendTextBtn.disabled = true;
        sendTextBtn.dataset.sending = 'true';
        const label = sendTextBtn.querySelector('#send-text-label') || sendTextBtn.querySelector('span');
        if (label) label.textContent = t.sendingText || 'Sending…';
      }
      if (textComposeError) textComposeError.style.display = 'none';

      try {
        const sessionId = getClientSessionId();
        const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/texts`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json',
            'X-Client-Session-ID': sessionId,
          },
          body: JSON.stringify({
            content: content,
            client_session_id: sessionId,
          }),
        });

        if (res.status === 201) {
          textInput.value = '';
          updateTextCounter();
          addRecentActivity('upload', 'Shared snippet', page === 'creator' ? 'Shared by Host' : 'Shared by Participant');
          const textsRes = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/texts`, {
            cache: 'no-store',
            headers: {
              'X-Client-Session-ID': sessionId,
            },
          });
          if (textsRes.ok) {
            const textsData = await textsRes.json();
            renderTextList(textsData.texts || []);
            const textListEl = document.getElementById('text-list');
            if (textListEl) {
              textListEl.scrollTop = textListEl.scrollHeight;
            }
          }
        } else {
          const errData = await res.json().catch(() => ({}));
          if (textComposeError) {
            textComposeError.textContent = errData.error || 'Failed to send text';
            textComposeError.style.display = 'block';
          }
        }
      } catch (err) {
        if (textComposeError) {
          textComposeError.textContent = 'Network error while sending text';
          textComposeError.style.display = 'block';
        }
      } finally {
        if (sendTextBtn) {
          delete sendTextBtn.dataset.sending;
          sendTextBtn.disabled = false;
          const label = sendTextBtn.querySelector('#send-text-label') || sendTextBtn.querySelector('span');
          if (label) label.textContent = t.sendText || 'Send Text';
        }
      }
    }

    if (sendTextBtn) {
      sendTextBtn.addEventListener('click', submitText);
    }
  }
})();

