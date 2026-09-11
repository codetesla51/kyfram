package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	kyfram "gitlab.com/codetesla51/kyfram"
)

// runView serves a live preview: scrub slider, play button, and hot
// reload for free — every request re-reads the script from disk, so
// saving the file updates the preview with no watcher and no restart.
func runView(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetInt("port")
	font, _ := cmd.Flags().GetString("font")
	if len(args) != 1 {
		errln("usage: kyfram view [--port n] [--font f] <script.lgv>")
		os.Exit(1)
	}
	script := args[0]

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// ponytail: ReplaceAll — viewPage has two {{SCRIPT}} (title + header)
		html := strings.ReplaceAll(viewPage, "{{SCRIPT}}", filepath.Base(script))
		fmt.Fprint(w, html)
	})
	http.HandleFunc("/font", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "font/ttf")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(kyfram.DisplayFont())
	})
	http.HandleFunc("/meta", func(w http.ResponseWriter, r *http.Request) {
		tl, err := loadScript(script, font)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{"duration": tl.Duration, "fps": tl.FPS})
	})
	http.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		tStr := r.URL.Query().Get("t")
		t, err := strconv.ParseFloat(tStr, 64)
		if err != nil {
			http.Error(w, "bad t", http.StatusBadRequest)
			return
		}
		tl, err := loadScript(script, font)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Encode to buffer first so we can return 500 before writing header
		// (prevents superfluous WriteHeader when png.Encode fails after header sent)
		var buf bytes.Buffer
		if err := png.Encode(&buf, kyfram.RenderFrame(tl, t).Image()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(buf.Bytes())
	})

	addr := "localhost:" + strconv.Itoa(port)
	fmt.Println("view:", "http://"+addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		errln("view:", err)
		os.Exit(1)
	}
}

var (
	cacheMu       sync.Mutex
	cacheTimeline kyfram.Timeline
	cacheMod      time.Time
	cachePath     string
	cacheFont     string
	cacheErr      error
	cacheErrMod   time.Time
)

// loadScript runs the file fresh: declarations in, Timeline out. Errors
// carry run/build context so the preview shows them instead of stale art.
// It caches by file modtime so scrubbing 30fps doesn't re-run Edge TTS (2s) every frame.
func loadScript(path, font string) (kyfram.Timeline, error) {
	// fast path: same file + same font + modtime unchanged -> cached
	if fi, err := os.Stat(path); err == nil {
		mod := fi.ModTime()
		cacheMu.Lock()
		if cachePath == path && cacheFont == font && cacheMod.Equal(mod) && !mod.IsZero() {
			tl, cErr := cacheTimeline, cacheErr
			cacheMu.Unlock()
			if cErr != nil {
				return kyfram.Timeline{}, cErr
			}
			return tl, nil
		}
		// also check cached error for same modtime to avoid spamming failing TTS
		if cachePath == path && cacheFont == font && cacheErrMod.Equal(mod) && cacheErr != nil {
			cErr := cacheErr
			cacheMu.Unlock()
			return kyfram.Timeline{}, cErr
		}
		cacheMu.Unlock()
	}
	source, err := kyfram.LoadScript(path)
	if err != nil {
		return kyfram.Timeline{}, fmt.Errorf("load: %w", err)
	}
	vm := newVM()
	var b kyfram.Builder
	b.Bind(vm)
	if err := vm.Run(string(source)); err != nil {
		err = fmt.Errorf("run: %w", err)
		if fi, serr := os.Stat(path); serr == nil {
			cacheMu.Lock()
			cachePath, cacheFont, cacheErr, cacheErrMod = path, font, err, fi.ModTime()
			cacheMu.Unlock()
		}
		return kyfram.Timeline{}, err
	}
	tl, err := b.Build()
	if err != nil {
		err = fmt.Errorf("build: %w", err)
		if fi, serr := os.Stat(path); serr == nil {
			cacheMu.Lock()
			cachePath, cacheFont, cacheErr, cacheErrMod = path, font, err, fi.ModTime()
			cacheMu.Unlock()
		}
		return kyfram.Timeline{}, err
	}
	if font != "" {
		tl.Font = font
	}
	if fi, err := os.Stat(path); err == nil {
		cacheMu.Lock()
		cachePath, cacheFont, cacheTimeline, cacheMod, cacheErr = path, font, tl, fi.ModTime(), nil
		cacheMu.Unlock()
	}
	return tl, nil
}

const viewPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>kyfram — {{SCRIPT}}</title>
<style>
@font-face {
  font-family: "Archivo Black";
  src: url("/font") format("truetype");
  font-display: swap;
}
* { box-sizing: border-box; }
:root {
  --bg: #0a0a18;
  --surface: #14141c;
  --surface-2: #1a1a28;
  --line: #2a2a38;
  --text: #ece8df;
  --muted: #8a877f;
  --gold: #e6c040;
  --gold-2: #f0d060;
  --red: #e64c40;
  --radius: 14px;
}
html, body { height: 100%; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, "Inter", Segoe UI, sans-serif;
  -webkit-font-smoothing: antialiased;
}
a { color: var(--gold); }
.wrap {
  max-width: 880px;
  margin: 0 auto;
  padding: 28px 20px 48px;
}
/* header — editorial, not SaaS card */
.top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 18px;
}
.brand {
  font-family: "Archivo Black", system-ui, sans-serif;
  font-size: 28px;
  letter-spacing: -0.02em;
  margin: 0;
  line-height: 1;
}
.brand span { color: var(--gold); }
.file {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  color: var(--muted);
  background: var(--surface);
  border: 1px solid var(--line);
  padding: 6px 10px;
  border-radius: 999px;
}
.dot {
  width: 7px; height: 7px; border-radius: 50%;
  background: var(--gold);
  box-shadow: 0 0 0 6px rgba(230,192,64,0.12);
}
/* stage — cinematic 16:9 card with subtle grain border */
.stage {
  background: #000;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  overflow: hidden;
  position: relative;
  box-shadow: 0 20px 40px rgba(0,0,0,0.45), 0 1px 0 rgba(255,255,255,0.06) inset;
}
.stage::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(to bottom, rgba(255,255,255,0.06), transparent 18%, transparent 82%, rgba(0,0,0,0.22));
}
.frame {
  width: 100%;
  aspect-ratio: 16 / 9;
  display: block;
  background: #000;
  object-fit: contain;
}
.frame--loading { opacity: 0.35; filter: blur(1px); }
/* transport — instrument, not form */
.transport {
  margin-top: 16px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 14px 16px 16px;
}
.time-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.time {
  font-family: "Archivo Black", system-ui, sans-serif;
  font-size: 42px;
  line-height: 1;
  color: var(--gold);
  letter-spacing: -0.03em;
}
.time small {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: var(--muted);
  letter-spacing: 0;
  margin-left: 10px;
  font-weight: 400;
}
.controls {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 14px;
}
.btn {
  appearance: none;
  border: 1px solid var(--gold);
  background: var(--gold);
  color: #16130a;
  font-weight: 800;
  font-size: 13px;
  letter-spacing: 0.02em;
  padding: 10px 18px;
  border-radius: 999px;
  cursor: pointer;
  transition: transform 120ms, background 120ms, border-color 120ms;
}
.btn:hover { background: var(--gold-2); border-color: var(--gold-2); transform: translateY(-1px); }
.btn:active { transform: translateY(0); }
.btn:focus-visible { outline: 2px solid var(--text); outline-offset: 2px; }
.scrub {
  flex: 1;
  -webkit-appearance: none;
  appearance: none;
  height: 10px;
  background: var(--surface-2);
  border: 1px solid var(--line);
  border-radius: 999px;
  outline: none;
  padding: 0 2px;
}
.scrub::-webkit-slider-runnable-track { height: 6px; background: transparent; border-radius: 999px; }
.scrub::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 18px; height: 18px;
  margin-top: -6px;
  background: var(--gold);
  border: 2px solid #1a1a28;
  border-radius: 50%;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(0,0,0,0.35);
}
.scrub::-moz-range-thumb {
  width: 18px; height: 18px;
  background: var(--gold);
  border: 2px solid #1a1a28;
  border-radius: 50%;
  cursor: pointer;
}
.scrub:focus-visible { outline: 2px solid var(--text); outline-offset: 2px; }
.meta {
  color: var(--muted);
  font-size: 13px;
  margin: 10px 2px 0;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(255,255,255,0.04);
  border: 1px solid var(--line);
  padding: 4px 8px;
  border-radius: 999px;
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.error {
  color: #ffd1cc;
  background: rgba(230,76,64,0.10);
  border: 1px solid rgba(230,76,64,0.35);
  border-radius: 10px;
  padding: 12px 14px;
  white-space: pre-wrap;
  word-break: break-word;
  margin-top: 14px;
  font-family: ui-monospace, monospace;
  font-size: 13px;
  line-height: 1.5;
}
.error:empty { display: none; }
.foot {
  color: var(--muted);
  font-size: 13px;
  margin-top: 18px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}
.kbd {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-bottom-width: 2px;
  padding: 2px 6px;
  border-radius: 6px;
  color: var(--text);
}
@media (max-width: 640px) {
  .time { font-size: 32px; }
  .wrap { padding: 18px 14px 36px; }
}
@media (prefers-reduced-motion: reduce) {
  .btn { transition: none; }
}
</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <h1 class="brand">kyfram<span>.</span></h1>
    <span class="file"><span class="dot" aria-hidden="true"></span>{{SCRIPT}}</span>
  </div>

  <div class="stage"><img id="f" class="frame" alt="preview frame" width="640" height="360"></div>

  <div class="transport">
    <div class="time-row">
      <div class="time" id="t">0.00<small id="tc">00:00 / 00:00</small></div>
      <div class="meta" id="m"></div>
    </div>
    <div class="controls">
      <button id="p" class="btn" type="button" aria-label="Play">Play</button>
      <input id="s" class="scrub" type="range" min="0" max="2" step="0.033" value="0" aria-label="scrub timeline">
    </div>
    <div id="e" class="error" role="alert"></div>
  </div>

  <div class="foot">
    <span><span class="kbd">Space</span> play/pause</span>
    <span>·</span>
    <span><span class="kbd">←</span> <span class="kbd">→</span> step frame</span>
    <span>·</span>
    <span>save file to hot-reload</span>
  </div>
</div>

<script>
let dur=2, fps=30, t=0, playing=false, timer=null, blobUrl=null;
const img=document.getElementById('f'), s=document.getElementById('s'),
      p=document.getElementById('p'), lbl=document.getElementById('t'),
      tc=document.getElementById('tc'), meta=document.getElementById('m'),
      err=document.getElementById('e');

function fmtTime(v){
  const m=Math.floor(v/60).toString().padStart(2,'0');
  const sec=(v%60).toFixed(2).padStart(5,'0');
  return m+':'+sec;
}

fetch('/meta').then(async r=>{
  const j=await r.json();
  if(!r.ok) throw new Error(j.error || 'meta failed');
  dur=j.duration; fps=j.fps;
  s.max=dur.toFixed(3);
  s.step=(1/fps).toFixed(5);
  meta.innerHTML = '<span class="pill">'+dur.toFixed(1)+'s</span><span class="pill">'+fps+' fps</span><span class="pill">'+Math.round(dur*fps)+' frames</span>';
  show();
}).catch(async e=>{
  err.textContent = e.message || 'Could not load script.';
});

async function show(){
  img.classList.add('frame--loading');
  try{
    const r=await fetch('/frame?t='+t.toFixed(3), {cache:'no-store'});
    if(!r.ok){
      const txt=await r.text();
      throw new Error(txt);
    }
    err.textContent='';
    const blob=await r.blob();
    const url=URL.createObjectURL(blob);
    if(blobUrl) URL.revokeObjectURL(blobUrl);
    blobUrl=url;
    img.src=url;
    lbl.firstChild.textContent=t.toFixed(2);
    tc.textContent=fmtTime(t)+' / '+fmtTime(dur);
    s.value=t.toFixed(3);
    img.classList.remove('frame--loading');
  }catch(e){
    err.textContent=e.message;
    img.classList.remove('frame--loading');
  }
}

function toggle(){
  playing=!playing;
  p.textContent=playing?'Pause':'Play';
  p.setAttribute('aria-label', playing?'Pause':'Play');
  if(playing){
    timer=setInterval(()=>{ t+=1/fps; if(t>dur) t=0; show(); }, 1000/fps);
  } else {
    clearInterval(timer);
  }
}
p.onclick=toggle;
s.oninput=()=>{ t=parseFloat(s.value); show(); };
document.addEventListener('keydown', e=>{
  if(e.code==='Space' && document.activeElement!==s){
    e.preventDefault();
    toggle();
  }
  if(e.code==='ArrowRight'){ t=Math.min(dur, t+1/fps); show(); }
  if(e.code==='ArrowLeft'){ t=Math.max(0, t-1/fps); show(); }
});
// hot-reload hint: meta re-fetched on focus (saving file updates next frame fetch)
window.addEventListener('focus', ()=>{
  fetch('/meta').then(r=>r.json()).then(m=>{
    if(m.duration && m.fps){
      dur=m.duration; fps=m.fps;
      s.max=dur.toFixed(3);
      s.step=(1/fps).toFixed(5);
      meta.innerHTML = '<span class="pill">'+dur.toFixed(1)+'s</span><span class="pill">'+fps+' fps</span><span class="pill">'+Math.round(dur*fps)+' frames</span>';
    }
  }).catch(()=>{});
});
</script>
</body>
</html>`
