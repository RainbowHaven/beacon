(function () {
  const close = document.querySelector("[data-receipt-back]");
  if (!close) {
    return;
  }

  function goBack(ev) {
    if (ev) {
      ev.preventDefault();
    }
    if (document.referrer) {
      history.back();
      return;
    }
    location.href = close.getAttribute("href");
  }

  close.addEventListener("click", goBack);
  document.addEventListener("keydown", function (ev) {
    if (ev.key !== "Escape") {
      return;
    }
    ev.preventDefault();
    close.click();
  });
})();
