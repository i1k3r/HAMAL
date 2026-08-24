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

    const downloadedKey = `hamal_downloaded_${token}`;
    let downloadedFiles = new Set();
    try {
      const stored = sessionStorage.getItem(downloadedKey);
      if (stored) {
        downloadedFiles = new Set(JSON.parse(stored));
      }
    } catch (e) {}

    function markFileDownloaded(fileId) {
      downloadedFiles.add(fileId);
      try {
        sessionStorage.setItem(downloadedKey, JSON.stringify(Array.from(downloadedFiles)));
      } catch (e) {}
    }

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
        seconds: "seconds"
      },
      tr: {
        executableWarning: "Potansiyel olarak çalıştırılabilir dosya. Yalnızca güvendiğiniz dosyaları açın veya yükleyin.",
        closeBtn: "Odayı Kapat",
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
        seconds: "saniye"
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
        seconds: "秒"
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
        seconds: "सेकंड"
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
        seconds: "segundos"
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
        seconds: "secondes"
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
        seconds: "ثوانٍ"
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
        seconds: "সেকেন্ড"
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
        seconds: "segundos"
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
        seconds: "сек."
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
        seconds: "سیکنڈ"
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
        seconds: "detik"
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
        seconds: "Sekunden"
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
        seconds: "秒"
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
        seconds: "सेकंद"
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
        seconds: "సెకన్లు"
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
        seconds: "seconden"
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
        seconds: "secondi"
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
        seconds: "초"
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
        seconds: "sekund"
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
      if (['bat', 'cmd'].includes(ext)) return 'script';
      if (['vbs', 'ps1', 'sh'].includes(ext)) return 'script';
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

    function getFileIconSVG(filename, contentType) {
      const cat = getFileCategory(filename);
      if (cat !== 'generic' && PIXEL_ICONS[cat]) {
        return PIXEL_ICONS[cat];
      }
      const type = (contentType || '').toLowerCase();
      if (type.startsWith('image/')) return PIXEL_ICONS.image;
      if (type.startsWith('video/')) return PIXEL_ICONS.video;
      if (type.startsWith('audio/')) return PIXEL_ICONS.audio;
      return PIXEL_ICONS.generic;
    }

    let closingTimerInterval = null;
    let activeClosingDeadline = null;
    let closingTargetTime = 0;

    function triggerClosingState(closeDeadlineStr, remainingSec) {
      const closingCard = document.getElementById('room-closing-card');
      const roomStatusCard = document.querySelector('.room-status-card');
      const dropzoneCard = document.querySelector('.dropzone-card');
      const bottomActions = document.querySelector('.participant-bottom-actions');
      const countdownEl = document.getElementById('closing-countdown');
      const badgeText = document.getElementById('closing-badge-text');
      const descText = document.getElementById('closing-card-desc');
      const secLabel = document.getElementById('closing-seconds-label');

      if (badgeText) badgeText.textContent = t.roomClosing || 'ROOM CLOSING';
      if (descText) descText.textContent = t.closingDesc || 'This transfer room will close automatically in';
      if (secLabel) secLabel.textContent = t.seconds || 'seconds';

      if (roomStatusCard) roomStatusCard.style.display = 'none';
      if (dropzoneCard) dropzoneCard.style.display = 'none';
      if (bottomActions) bottomActions.style.display = 'none';
      if (closingCard) closingCard.style.display = 'flex';

      // If timer is already running for this exact server deadline, preserve uninterrupted 1000ms loop
      if (closingTimerInterval && activeClosingDeadline && closeDeadlineStr && activeClosingDeadline === closeDeadlineStr) {
        return;
      }

      activeClosingDeadline = closeDeadlineStr || '';

      if (closingTimerInterval) {
        clearInterval(closingTimerInterval);
        closingTimerInterval = null;
      }

      // Calculate target time synchronized with local clock
      const serverDeadlineMs = closeDeadlineStr ? new Date(closeDeadlineStr).getTime() : NaN;
      if (!isNaN(serverDeadlineMs) && typeof remainingSec === 'number' && remainingSec > 0) {
        closingTargetTime = Date.now() + remainingSec * 1000;
      } else if (!isNaN(serverDeadlineMs)) {
        closingTargetTime = serverDeadlineMs;
      } else {
        const sec = (typeof remainingSec === 'number' && remainingSec > 0) ? remainingSec : 10;
        closingTargetTime = Date.now() + sec * 1000;
      }

      function renderCountdownTick() {
        const now = Date.now();
        const remaining = Math.max(0, Math.ceil((closingTargetTime - now) / 1000));
        if (countdownEl) {
          countdownEl.textContent = remaining;
        }
        if (remaining <= 0) {
          if (closingTimerInterval) {
            clearInterval(closingTimerInterval);
            closingTimerInterval = null;
          }
          showInactive(t.closedTitle || 'Room Closed', t.closedMsg || 'This temporary transfer room has been closed.');
        }
      }

      renderCountdownTick();
      closingTimerInterval = setInterval(renderCountdownTick, 1000);
    }

    function showInactive(title, message) {
      isTerminated = true;
      if (pollTimer) clearTimeout(pollTimer);
      if (closingTimerInterval) {
        clearInterval(closingTimerInterval);
        closingTimerInterval = null;
      }
      activeClosingDeadline = null;
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
          if (data.status === 'closing') {
            triggerClosingState(data.close_deadline, data.closing_remaining_seconds);
          } else if (data.status === 'closed') {
            showInactive(t.closedTitle || 'Room Closed', t.closedMsg || 'This room was closed.');
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
        if (filesRes.status === 404 || filesRes.status === 410) {
          showInactive(t.closedTitle || 'Room Closed', t.closedMsg || 'This temporary room is no longer accessible.');
          return;
        }
        if (filesRes.ok) {
          const filesData = await filesRes.json();
          if (filesData.status === 'closing') {
            triggerClosingState(filesData.close_deadline, filesData.closing_remaining_seconds);
          } else if (filesData.status === 'closed') {
            showInactive(t.closedTitle || 'Room Closed', t.closedMsg || 'This room was closed.');
            return;
          }
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

        const isDownloaded = downloadedFiles.has(file.file_id);
        const downloadLink = document.createElement('a');
        downloadLink.href = `/api/v1/rooms/${encodeURIComponent(token)}/files/${encodeURIComponent(file.file_id)}`;
        downloadLink.download = file.filename;

        if (isDownloaded) {
          downloadLink.className = 'btn btn-secondary btn-sm btn-saved';
          downloadLink.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
            Saved
          `;
        } else {
          downloadLink.className = 'btn btn-secondary btn-sm btn-download';
          downloadLink.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
              <polyline points="7 10 12 15 17 10"></polyline>
              <line x1="12" y1="15" x2="12" y2="3"></line>
            </svg>
            Download
          `;
          downloadLink.addEventListener('click', () => {
            markFileDownloaded(file.file_id);
            downloadLink.className = 'btn btn-secondary btn-sm btn-saved';
            downloadLink.innerHTML = `
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12"></polyline>
              </svg>
              Saved
            `;
          });
        }

        if (isPotentiallyExecutable(file.filename)) {
          const warnSpan = document.createElement('span');
          warnSpan.className = 'badge-exec-warning';
          warnSpan.tabIndex = 0;
          warnSpan.setAttribute('role', 'tooltip');
          warnSpan.setAttribute('aria-label', t.executableWarning);
          warnSpan.title = t.executableWarning;
          warnSpan.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
            <span class="warning-tooltip">${t.executableWarning}</span>
          `;
          actions.appendChild(warnSpan);
        }

        actions.appendChild(downloadLink);

        item.appendChild(mainDiv);
        item.appendChild(actions);
        fileListEl.appendChild(item);
      });
    }

    // Attach listeners and saved state to server pre-rendered file items
    function initPreRenderedFiles() {
      const items = document.querySelectorAll('.file-item');
      items.forEach((item) => {
        const fileId = item.dataset.fileId;
        const link = item.querySelector('.btn-download, .btn-saved');
        const nameEl = item.querySelector('.file-name');
        const iconEl = item.querySelector('.file-type-icon');
        const actionsEl = item.querySelector('.file-actions');
        const filename = nameEl ? nameEl.textContent : '';

        if (iconEl && filename) {
          iconEl.innerHTML = getFileIconSVG(filename);
        }

        if (actionsEl && link && filename && isPotentiallyExecutable(filename) && !actionsEl.querySelector('.badge-exec-warning')) {
          const warnSpan = document.createElement('span');
          warnSpan.className = 'badge-exec-warning';
          warnSpan.tabIndex = 0;
          warnSpan.setAttribute('role', 'tooltip');
          warnSpan.setAttribute('aria-label', t.executableWarning);
          warnSpan.title = t.executableWarning;
          warnSpan.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
              <line x1="12" y1="9" x2="12" y2="13"></line>
              <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>
            <span class="warning-tooltip">${t.executableWarning}</span>
          `;
          actionsEl.insertBefore(warnSpan, link);
        }

        if (!fileId || !link) return;

        if (downloadedFiles.has(fileId)) {
          link.className = 'btn btn-secondary btn-sm btn-saved';
          link.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
            Saved
          `;
        } else {
          link.addEventListener('click', () => {
            markFileDownloaded(fileId);
            link.className = 'btn btn-secondary btn-sm btn-saved';
            link.innerHTML = `
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12"></polyline>
              </svg>
              Saved
            `;
          });
        }
      });
    }
    initPreRenderedFiles();

    // Check if room is loaded in closing state
    const initialStatus = document.body.dataset.status;
    const initialCloseDeadline = document.body.dataset.closeDeadline;
    const initialClosingSec = parseInt(document.body.dataset.closingSeconds, 10);
    if (initialStatus === 'closing') {
      triggerClosingState(initialCloseDeadline, initialClosingSec);
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
    // 7B. Participant Close Room Handler with Confirmation & Multi-Language Support
    // --------------------------------------------------------------------------
    const participantCloseBtn = document.getElementById('participant-close-btn');
    const closeConfirmModal = document.getElementById('close-confirm-modal');
    const confirmModalTitle = document.getElementById('confirm-modal-title');
    const confirmModalDesc = document.getElementById('confirm-modal-desc');
    const cancelCloseBtn = document.getElementById('cancel-close-btn');
    const confirmCloseBtn = document.getElementById('confirm-close-btn');

    // Localize modal & button strings
    const participantCloseText = document.getElementById('participant-close-text');
    if (participantCloseText) {
      participantCloseText.textContent = t.closeBtn;
    } else if (participantCloseBtn) {
      participantCloseBtn.textContent = t.closeBtn;
    }
    if (confirmModalTitle) confirmModalTitle.textContent = t.confirmTitle;
    if (confirmModalDesc) confirmModalDesc.textContent = t.confirmDesc;
    if (cancelCloseBtn) cancelCloseBtn.textContent = t.cancelBtn;
    if (confirmCloseBtn) confirmCloseBtn.textContent = t.confirmBtn;

    if (participantCloseBtn && closeConfirmModal) {
      participantCloseBtn.addEventListener('click', () => {
        closeConfirmModal.style.display = 'flex';
      });

      if (cancelCloseBtn) {
        cancelCloseBtn.addEventListener('click', () => {
          closeConfirmModal.style.display = 'none';
        });
      }

      closeConfirmModal.addEventListener('click', (e) => {
        if (e.target === closeConfirmModal) {
          closeConfirmModal.style.display = 'none';
        }
      });

      if (confirmCloseBtn) {
        confirmCloseBtn.addEventListener('click', async () => {
          confirmCloseBtn.disabled = true;
          confirmCloseBtn.textContent = t.closing;
          if (cancelCloseBtn) cancelCloseBtn.disabled = true;

          try {
            const res = await fetch(`/api/v1/rooms/${encodeURIComponent(token)}/close`, {
              method: 'POST',
            });
            closeConfirmModal.style.display = 'none';
            if (res.ok) {
              const closeData = await res.json().catch(() => ({}));
              if (closeData.status === 'closing') {
                triggerClosingState(closeData.close_deadline, closeData.closing_remaining_seconds || 10);
              } else {
                showInactive(t.closedTitle, t.closedMsg);
              }
            } else if (res.status === 404 || res.status === 410) {
              showInactive(t.closedTitle, t.closedMsg);
            } else {
              const errData = await res.json().catch(() => ({}));
              alert(errData.error || t.closeError);
              confirmCloseBtn.disabled = false;
              confirmCloseBtn.textContent = t.confirmBtn;
              if (cancelCloseBtn) cancelCloseBtn.disabled = false;
            }
          } catch (err) {
            closeConfirmModal.style.display = 'none';
            alert(t.networkError);
            confirmCloseBtn.disabled = false;
            confirmCloseBtn.textContent = t.confirmBtn;
            if (cancelCloseBtn) cancelCloseBtn.disabled = false;
          }
        });
      }
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
