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

	kyfram "github.com/codetesla51/kyfram"
	"github.com/spf13/cobra"
)

// runView serves a live preview: scrub slider, play button, and hot
// reload for free — every request re-reads the script from disk, so
// saving the file updates the preview with no watcher and no restart.
func runView(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetInt("port")
	font, _ := cmd.Flags().GetString("font")
	width, _ := cmd.Flags().GetInt("width")
	height, _ := cmd.Flags().GetInt("height")
	if len(args) != 1 {
		errln("usage: kyfram view [--port n] [--font f] [--width n] [--height n] <script.lgv>")
		os.Exit(1)
	}
	if width < 0 || height < 0 {
		errln("width and height must be positive")
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
		tl, err := loadScript(script, font, width, height)
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
		tl, err := loadScript(script, font, width, height)
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
func loadScript(path, font string, width, height int) (kyfram.Timeline, error) {
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
	b.SetCanvas(width, height)
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
	if width > 0 {
		tl.Width = width
	}
	if height > 0 {
		tl.Height = height
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
  font-family: "kyfram display";
  src: url("/font") format("truetype");
  font-display: swap;
}
* { box-sizing: border-box; }
:root {
  /* kyfram-site palette — warm zinc, monochrome */
  --bg: #0d0d0c;
  --surface: #171816;
  --surface-2: #232421;
  --line: #343531;
  --line-strong: #4b4c46;
  --text: #f5f5f4;
  --muted: #8f8f86;
  --muted-2: #8a8a80;
  --error: #d05448;
  --radius: 14px;
}
html, body { height: 100%; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
  -webkit-font-smoothing: antialiased;
}
a { color: var(--text); }
.wrap { max-width: 880px; margin: 0 auto; padding: 28px 20px 48px; }
.top {
  display: flex; align-items: center; justify-content: space-between;
  gap: 16px; flex-wrap: wrap; margin-bottom: 18px;
}
.brand {
  font-family: "kyfram display", system-ui, sans-serif;
  font-size: 26px; letter-spacing: -0.02em; margin: 0; line-height: 1;
}
.brand span { color: var(--muted-2); }
.file {
  display: inline-flex; align-items: center; gap: 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px; color: var(--muted);
  background: var(--surface); border: 1px solid var(--line);
  padding: 6px 11px; border-radius: 999px;
}
.file::before {
  content: ""; width: 7px; height: 7px; border-radius: 50%;
  background: var(--text);
}
.stage {
  background: #000; border: 1px solid var(--line);
  border-radius: var(--radius); overflow: hidden; position: relative;
}
.frame {
  width: 100%; aspect-ratio: 16 / 9; display: block;
  background: #000; object-fit: contain;
}
.frame--loading { opacity: 0.35; filter: blur(1px); }
.transport {
  margin-top: 16px; background: var(--surface);
  border: 1px solid var(--line); border-radius: var(--radius);
  padding: 16px 16px 18px;
}
.time-row {
  display: flex; align-items: baseline; justify-content: space-between;
  gap: 12px; flex-wrap: wrap;
}
.time {
  font-family: "kyfram display", system-ui, sans-serif;
  font-size: 40px; line-height: 1; color: var(--text);
  letter-spacing: -0.02em;
}
.time small {
  font-family: ui-monospace, monospace; font-size: 12px;
  color: var(--muted); letter-spacing: 0; margin-left: 10px; font-weight: 400;
}
.meta { color: var(--muted); font-size: 13px; display: flex; gap: 8px; flex-wrap: wrap; }
.pill {
  display: inline-flex; align-items: center; gap: 6px;
  background: var(--surface-2); border: 1px solid var(--line);
  padding: 4px 9px; border-radius: 999px;
  font-family: ui-monospace, monospace; font-size: 12px; color: var(--muted);
}
.controls { display: flex; align-items: center; gap: 10px; margin-top: 14px; flex-wrap: wrap; }
.btn {
  appearance: none; border: 1px solid var(--text); background: var(--text);
  color: var(--bg); font-weight: 700; font-size: 13px; letter-spacing: 0.01em;
  min-height: 44px; padding: 0 18px; border-radius: 999px; cursor: pointer;
  transition: transform 120ms, background 120ms, border-color 120ms;
}
.btn:hover { background: #ffffff; border-color: #ffffff; }
.btn:active { transform: translateY(1px); }
.btn:focus-visible { outline: 2px solid var(--text); outline-offset: 2px; }
.btn.ghost { background: transparent; color: var(--text); border-color: var(--line-strong); }
.btn.ghost:hover { background: var(--surface-2); border-color: var(--text); }
.btn.ghost[aria-pressed="true"] { background: var(--text); color: var(--bg); border-color: var(--text); }
.ruler { position: relative; height: 14px; margin: 10px 4px 0; }
.ruler .tick { position: absolute; top: 0; width: 1px; height: 7px; background: var(--line); }
.ruler .tick.major { height: 10px; background: var(--line-strong); }
.ruler .edge {
  position: absolute; top: 9px; font-family: ui-monospace, monospace;
  font-size: 10px; color: var(--muted-2);
}
.ruler .edge.l { left: 0; }
.ruler .edge.r { right: 0; }
.scrub {
  width: 100%; -webkit-appearance: none; appearance: none;
  height: 24px; margin: 0; background: transparent; border: none;
  outline: none; cursor: pointer;
}
.scrub::-webkit-slider-runnable-track {
  height: 10px; background: var(--surface-2);
  border: 1px solid var(--line); border-radius: 999px;
}
.scrub::-webkit-slider-thumb {
  -webkit-appearance: none; width: 17px; height: 17px; margin-top: -4.5px;
  background: var(--text); border: 1px solid var(--bg); border-radius: 50%;
}
.scrub::-moz-range-track {
  height: 10px; background: var(--surface-2);
  border: 1px solid var(--line); border-radius: 999px;
}
.scrub::-moz-range-thumb { width: 17px; height: 17px; background: var(--text); border: 1px solid var(--bg); border-radius: 50%; }
.scrub:focus-visible { outline: 2px solid var(--text); outline-offset: 2px; }
.error {
  color: #f2b6ae; background: rgba(208,84,72,0.12);
  border: 1px solid rgba(208,84,72,0.45);
  border-radius: 10px; padding: 12px 14px; white-space: pre-wrap; word-break: break-word;
  margin-top: 14px; font-family: ui-monospace, monospace; font-size: 13px; line-height: 1.5;
}
.error:empty { display: none; }
.foot {
  color: var(--muted); font-size: 13px; margin-top: 18px;
  display: flex; gap: 8px; flex-wrap: wrap; align-items: center;
}
.kbd {
  font-family: ui-monospace, monospace; font-size: 11px;
  background: var(--surface); border: 1px solid var(--line);
  border-bottom-width: 2px; padding: 2px 6px; border-radius: 6px; color: var(--text);
}
@media (max-width: 640px) {
  .time { font-size: 30px; }
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
    <span class="file">{{SCRIPT}}</span>
  </div>

  <div class="stage" id="stage"><img id="f" class="frame" alt="preview frame" width="640" height="360"></div>

  <div class="transport">
    <div class="time-row">
      <div class="time" id="t">0.00<small id="tc">00:00 / 00:00 &middot; f0</small></div>
      <div class="meta" id="m"></div>
    </div>
    <div class="ruler" id="ruler">
      <span class="edge l">0</span><span class="edge r" id="durLbl">-</span>
    </div>
    <input id="s" class="scrub" type="range" min="0" max="2" step="0.033" value="0" aria-label="scrub timeline">
    <div class="controls">
      <button id="p" class="btn" type="button" aria-label="Play">Play</button>
      <button id="lp" class="btn ghost" type="button" aria-pressed="true" aria-label="Loop">Loop</button>
      <button id="dl" class="btn ghost" type="button" aria-label="Download current frame as PNG">PNG</button>
      <button id="fs" class="btn ghost" type="button" aria-label="Fullscreen the stage">Full</button>
    </div>
    <div id="e" class="error" role="alert"></div>
  </div>

  <div class="foot">
    <span><span class="kbd">Space</span> play/pause</span>
    <span>&middot;</span>
    <span><span class="kbd">&larr;</span> <span class="kbd">&rarr;</span> step frame</span>
    <span>&middot;</span>
    <span><span class="kbd">Home</span> <span class="kbd">End</span> jump</span>
    <span>&middot;</span>
    <span><span class="kbd">F</span> fullscreen</span>
    <span>&middot;</span>
    <span>save file to hot-reload</span>
  </div>
</div>

<script>
let dur=2, fps=30, t=0, playing=false, loop=true, timer=null, blobUrl=null;
const base = '{{SCRIPT}}'.replace(/\.lgv$/,'');
const img=document.getElementById('f'), s=document.getElementById('s'),
      p=document.getElementById('p'), lp=document.getElementById('lp'),
      dl=document.getElementById('dl'), fs=document.getElementById('fs'),
      lbl=document.getElementById('t'), tc=document.getElementById('tc'),
      meta=document.getElementById('m'), err=document.getElementById('e'),
      ruler=document.getElementById('ruler'), durLbl=document.getElementById('durLbl');

function fmtTime(v){
  const m=Math.floor(v/60).toString().padStart(2,'0');
  const sec=(v%60).toFixed(2).padStart(5,'0');
  return m+':'+sec;
}

function setMeta(d, f){
  dur=d; fps=f;
  s.max=dur.toFixed(3);
  s.step=(1/fps).toFixed(5);
  meta.innerHTML='<span class="pill">'+dur.toFixed(1)+'s</span><span class="pill">'+fps+' fps</span><span class="pill">'+Math.round(dur*fps)+' frames</span>';
  buildRuler();
}

function buildRuler(){
  ruler.querySelectorAll('.tick').forEach(n=>n.remove());
  durLbl.textContent=dur.toFixed(1)+'s';
  if(!(dur>0)) return;
  const step = dur<=5 ? 0.5 : dur<=20 ? 1 : dur<=60 ? 2 : 5;
  for(let tv=step; tv<dur; tv+=step){
    const d=document.createElement('span');
    d.className='tick'+(Math.abs(tv-Math.round(tv))<1e-9 ? ' major' : '');
    d.style.left=(tv/dur*100).toFixed(3)+'%';
    ruler.appendChild(d);
  }
}

async function show(){
  img.classList.add('frame--loading');
  try{
    const r=await fetch('/frame?t='+t.toFixed(3), {cache:'no-store'});
    if(!r.ok) throw new Error(await r.text());
    err.textContent='';
    const blob=await r.blob();
    const url=URL.createObjectURL(blob);
    if(blobUrl) URL.revokeObjectURL(blobUrl);
    blobUrl=url;
    img.src=url;
    lbl.firstChild.textContent=t.toFixed(2);
    tc.textContent=fmtTime(t)+' / '+fmtTime(dur)+' \u00b7 f'+Math.round(t*fps);
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
    timer=setInterval(()=>{
      t+=1/fps;
      if(t>dur){
        if(loop){ t=0; } else { t=dur; toggle(); return; }
      }
      show();
    }, 1000/fps);
  } else {
    clearInterval(timer);
  }
}
p.onclick=toggle;
s.oninput=()=>{ t=parseFloat(s.value); show(); };
lp.onclick=()=>{ loop=!loop; lp.setAttribute('aria-pressed', String(loop)); };
dl.onclick=()=>{
  if(!blobUrl) return;
  const a=document.createElement('a');
  a.href=blobUrl;
  a.download=base+'-f'+Math.round(t*fps)+'.png';
  a.click();
};
fs.onclick=()=>{
  const st=document.getElementById('stage');
  if(!document.fullscreenEnabled) return;
  if(document.fullscreenElement) document.exitFullscreen();
  else st.requestFullscreen().catch(()=>{});
};

document.addEventListener('keydown', e=>{
  const onCtrl = document.activeElement===s || document.activeElement.tagName==='BUTTON';
  if(e.code==='Space' && !onCtrl){ e.preventDefault(); toggle(); }
  if(onCtrl) return;
  const st=1/fps;
  if(e.code==='ArrowRight'){ t=Math.min(dur, t+st); show(); }
  else if(e.code==='ArrowLeft'){ t=Math.max(0, t-st); show(); }
  else if(e.code==='Home'){ t=0; show(); }
  else if(e.code==='End'){ t=dur; show(); }
  else if(e.code==='KeyF'){ fs.onclick(); }
});

fetch('/meta').then(async r=>{
  const j=await r.json();
  if(!r.ok) throw new Error(j.error || 'meta failed');
  setMeta(j.duration, j.fps);
  show();
  // ?autoplay=1 is handy for opening a wall of previews at once.
  if(new URLSearchParams(location.search).get('autoplay')==='1') toggle();
}).catch(e=>{ err.textContent=e.message || 'Could not load script.'; });

// hot reload: on focus, re-fetch meta; duration/fps changes apply live
window.addEventListener('focus', ()=>{
  fetch('/meta').then(r=>r.json()).then(j=>{
    if(j.duration && j.fps && (j.duration!==dur || j.fps!==fps)){
      setMeta(j.duration, j.fps);
      if(t>dur) { t=dur; show(); }
    }
  }).catch(()=>{});
});
</script>
</body>
</html>`
