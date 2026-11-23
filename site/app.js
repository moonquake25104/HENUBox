const root = document.documentElement;
const savedThemeKey = "henubox-site-theme";
const systemTheme = matchMedia("(prefers-color-scheme: dark)");
const themeButton = document.querySelector("#theme-toggle");
const themeMeta = document.querySelector('meta[name="theme-color"]');

try {
  const theme = localStorage.getItem(savedThemeKey);
  if (theme === "light" || theme === "dark") root.dataset.theme = theme;
} catch {
  // Follow the system preference when storage is unavailable.
}

function isDark() {
  return root.dataset.theme ? root.dataset.theme === "dark" : systemTheme.matches;
}

function updateThemeButton() {
  const dark = isDark();
  themeButton.setAttribute("aria-label", dark ? "切换到浅色模式" : "切换到深色模式");
  themeButton.setAttribute("aria-pressed", String(dark));
  themeMeta.content = dark ? "#0f1c19" : "#f6f7f1";
}

themeButton.hidden = false;
updateThemeButton();
systemTheme.addEventListener("change", updateThemeButton);
themeButton.addEventListener("click", () => {
  root.dataset.theme = isDark() ? "light" : "dark";
  try {
    localStorage.setItem(savedThemeKey, root.dataset.theme);
  } catch {
    // The current page still reflects the selection.
  }
  updateThemeButton();
});

const header = document.querySelector(".site-header");
const paintHeader = () => header.classList.toggle("is-scrolled", window.scrollY > 8);
paintHeader();
window.addEventListener("scroll", paintHeader, { passive: true });

const options = document.querySelectorAll(".screen-options a");
const galleryImage = document.querySelector("#screen-image");
const imageLink = document.querySelector("#screen-image-link");
const enlarge = document.querySelector("#enlarge-screen");
const screenTitle = document.querySelector("#screen-title");
const screenDescription = document.querySelector("#screen-description");
const lightbox = document.querySelector("#lightbox");
const lightboxImage = document.querySelector("#lightbox-image");
const lightboxCaption = document.querySelector("#lightbox-caption");
const closeButton = lightbox.querySelector(".lightbox-close");

function isPrimaryClick(event) {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey;
}

options.forEach(option => {
  option.addEventListener("click", event => {
    if (!isPrimaryClick(event)) return;
    event.preventDefault();
    document.querySelector('.screen-options [aria-current="true"]').removeAttribute("aria-current");
    option.setAttribute("aria-current", "true");
    galleryImage.src = option.href;
    galleryImage.alt = option.dataset.alt;
    imageLink.href = option.href;
    imageLink.setAttribute("aria-label", `放大${option.dataset.title}截图`);
    enlarge.href = option.href;
    screenTitle.textContent = option.dataset.title;
    screenDescription.textContent = option.dataset.description;
  });
});

let openedFrom;
for (const link of [enlarge, imageLink]) {
  link.addEventListener("click", event => {
    if (!isPrimaryClick(event) || typeof lightbox.showModal !== "function") return;
    event.preventDefault();
    openedFrom = link;
    lightboxImage.src = link.href;
    lightboxImage.alt = galleryImage.alt;
    lightboxCaption.textContent = screenTitle.textContent;
    lightbox.showModal();
    closeButton.focus();
  });
}

lightbox.addEventListener("click", event => {
  if (event.target !== lightbox) return;
  const { left, right, top, bottom } = lightbox.getBoundingClientRect();
  if (event.clientX < left || event.clientX > right || event.clientY < top || event.clientY > bottom) {
    lightbox.close();
  }
});
lightbox.addEventListener("close", () => openedFrom?.focus({ preventScroll: true }));
