// Optional client-side image resize before receipt upload (keeps PDFs untouched).
const MAX_EDGE = 1600;
const JPEG_QUALITY = 0.82;

const form = document.getElementById("expense-form");
const input = document.getElementById("receipt");
const errEl = document.getElementById("expense-error");

if (form && input) {
  form.addEventListener("submit", async (e) => {
    const file = input.files && input.files[0];
    if (!file || !file.type.startsWith("image/")) {
      return;
    }
    e.preventDefault();
    errEl.hidden = true;
    try {
      const resized = await resizeImage(file);
      const dt = new DataTransfer();
      dt.items.add(resized);
      input.files = dt.files;
      form.submit();
    } catch (err) {
      errEl.textContent = err.message || String(err);
      errEl.hidden = false;
    }
  });
}

function resizeImage(file) {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      let { width, height } = img;
      const scale = Math.min(1, MAX_EDGE / Math.max(width, height));
      width = Math.round(width * scale);
      height = Math.round(height * scale);
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext("2d");
      if (!ctx) {
        reject(new Error("Could not resize image"));
        return;
      }
      ctx.drawImage(img, 0, 0, width, height);
      canvas.toBlob(
        (blob) => {
          if (!blob) {
            reject(new Error("Could not encode image"));
            return;
          }
          const name = file.name.replace(/\.\w+$/, "") + ".jpg";
          resolve(new File([blob], name, { type: "image/jpeg", lastModified: Date.now() }));
        },
        "image/jpeg",
        JPEG_QUALITY
      );
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("Could not load image"));
    };
    img.src = url;
  });
}
