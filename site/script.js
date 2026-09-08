// Copy buttons: copy the target block, flip the icon briefly as the answer.
document.querySelectorAll(".copy").forEach((btn) => {
  btn.addEventListener("click", async () => {
    const el = document.getElementById(btn.dataset.copy);
    if (!el) return;
    try {
      await navigator.clipboard.writeText(el.innerText);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(el);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      document.execCommand("copy");
      sel.removeAllRanges();
    }
    const icon = btn.querySelector("i");
    if (!icon) return;
    icon.className = "ph ph-check";
    setTimeout(() => {
      icon.className = "ph ph-copy";
    }, 1200);
  });
});

// Tabs: one panel visible, aria kept honest.
document.querySelectorAll(".tabs").forEach((bar) => {
  const tabs = bar.querySelectorAll(".tab");
  const panels = bar.parentElement.querySelectorAll(".tab-panel");
  tabs.forEach((tab) => {
    tab.addEventListener("click", () => {
      tabs.forEach((t) => {
        t.classList.toggle("active", t === tab);
        t.setAttribute("aria-selected", t === tab ? "true" : "false");
      });
      panels.forEach((p) => p.classList.toggle("active", p.dataset.panel === tab.dataset.tab));
    });
  });
});

// Syntax highlighting for the script blocks.
if (window.hljs) hljs.highlightAll();

// Showcase clips: play muted when mostly visible, pause offscreen.
// Tap toggles. Respects reduced motion by staying paused.
(function () {
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const clips = document.querySelectorAll("video[data-autoplay]");
  if (!clips.length) return;
  clips.forEach((v) => {
    v.addEventListener("click", () => {
      if (v.paused) v.play().catch(() => {});
      else v.pause();
    });
  });
  if (reduce || !("IntersectionObserver" in window)) return;
  const io = new IntersectionObserver(
    (entries) => {
      entries.forEach((e) => {
        const v = e.target;
        if (e.intersectionRatio >= 0.5) v.play().catch(() => {});
        else v.pause();
      });
    },
    { threshold: [0, 0.5, 1] }
  );
  clips.forEach((v) => io.observe(v));
})();

// Hero demo: the intro.lgv ball, interpolated live.
// x lerps 100 -> 540 over 2s with easeInOut, then loops. Same math as Interpolate.
(function () {
  const canvas = document.getElementById("demo-canvas");
  const time = document.getElementById("demo-time");
  const toggle = document.getElementById("demo-toggle");
  if (!canvas) return;
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const ctx = canvas.getContext("2d");
  const DUR = 2;
  const easeInOut = (f) => (f < 0.5 ? 2 * f * f : 1 - Math.pow(-2 * f + 2, 2) / 2);

  let playing = !reduce;
  let start = performance.now();

  function frame(now) {
    const t = ((now - start) / 1000) % DUR;
    const f = easeInOut(t / DUR);
    const x = 100 + (540 - 100) * f;

    ctx.fillStyle = "#191920";
    ctx.fillRect(0, 0, 640, 360);
    ctx.strokeStyle = "#404040";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(320, 100);
    ctx.lineTo(320, 260);
    ctx.stroke();
    ctx.fillStyle = "rgba(230, 76, 64, 0.25)";
    for (let i = 0; i < 24; i++) {
      const pt = t - i * 0.02;
      if (pt < 0) break;
      const px = 100 + (540 - 100) * easeInOut(pt / DUR);
      ctx.beginPath();
      ctx.arc(px, 180, 40 * (1 - i / 32), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.fillStyle = "#e64c40";
    ctx.beginPath();
    ctx.arc(x, 180, 40, 0, Math.PI * 2);
    ctx.fill();

    if (time) time.textContent = t.toFixed(2) + " / " + DUR.toFixed(2);
    if (playing) requestAnimationFrame(frame);
  }

  function setIcon() {
    if (!toggle) return;
    toggle.innerHTML = playing
      ? '<i class="ph-fill ph-pause" aria-hidden="true"></i>'
      : '<i class="ph-fill ph-play" aria-hidden="true"></i>';
    toggle.setAttribute("aria-label", playing ? "Pause demo" : "Play demo");
  }

  if (toggle) {
    toggle.addEventListener("click", () => {
      playing = !playing;
      setIcon();
      if (playing) {
        start = performance.now();
        requestAnimationFrame(frame);
      }
    });
  }
  setIcon();
  requestAnimationFrame(frame);
})();
