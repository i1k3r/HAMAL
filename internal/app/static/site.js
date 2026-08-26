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