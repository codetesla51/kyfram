package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"strconv"

	kyfram "github.com/codetesla51/kyfram"
)

// runView serves a live preview: scrub slider, play button, and hot
// reload for free — every request re-reads the script from disk, so
// saving the file updates the preview with no watcher and no restart.
func runView(args []string) {
	fs := flag.NewFlagSet("view", flag.ExitOnError)
	port := fs.Int("port", 8080, "preview server port")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "flags:", err)
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram view [--port n] <script.lgv>")
		os.Exit(1)
	}
	script := fs.Arg(0)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, viewPage)
	})
	http.HandleFunc("/meta", func(w http.ResponseWriter, r *http.Request) {
		tl, err := loadScript(script)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"duration": tl.Duration, "fps": tl.FPS})
	})
	http.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		t, err := strconv.ParseFloat(r.URL.Query().Get("t"), 64)
		if err != nil {
			http.Error(w, "bad t", http.StatusBadRequest)
			return
		}
		tl, err := loadScript(script)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, kyfram.RenderFrame(tl, t).Image()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	addr := "localhost:" + strconv.Itoa(*port)
	fmt.Println("view:", "http://"+addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "view:", err)
		os.Exit(1)
	}
}

// loadScript runs the file fresh: declarations in, Timeline out. Errors
// carry run/build context so the preview shows them instead of stale art.
func loadScript(path string) (kyfram.Timeline, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return kyfram.Timeline{}, fmt.Errorf("read: %w", err)
	}
	vm := newVM()
	var b kyfram.Builder
	b.Bind(vm)
	if err := vm.Run(string(source)); err != nil {
		return kyfram.Timeline{}, fmt.Errorf("run: %w", err)
	}
	tl, err := b.Build()
	if err != nil {
		return kyfram.Timeline{}, fmt.Errorf("build: %w", err)
	}
	return tl, nil
}

const viewPage = `<!doctype html>
<html><body style="background:#111;color:#eee;font-family:sans-serif;text-align:center">
<h2>kyfram preview</h2>
<img id="f" style="max-width:90vw"><br>
<input id="s" type="range" min="0" max="2" step="0.033" value="0" style="width:min(640px,90vw)">
<button id="p">play</button> <span id="t">0.00s</span>
<pre id="e" style="color:#f66"></pre>
<script>
let dur=2, fps=30, t=0, playing=false, timer=null;
const img=document.getElementById('f'), s=document.getElementById('s'),
      p=document.getElementById('p'), lbl=document.getElementById('t'),
      err=document.getElementById('e');
fetch('/meta').then(r=>r.json()).then(m=>{dur=m.duration;fps=m.fps;s.max=dur;s.step=1/fps;show();});
async function show(){
  const r=await fetch('/frame?t='+t.toFixed(3));
  if(!r.ok){err.textContent=await r.text();return;}
  err.textContent='';
  img.src=URL.createObjectURL(await r.blob());
  lbl.textContent=t.toFixed(2)+'s'; s.value=t;
}
s.oninput=()=>{t=parseFloat(s.value);show();};
p.onclick=()=>{
  playing=!playing; p.textContent=playing?'pause':'play';
  if(playing){timer=setInterval(()=>{t+=1/fps;if(t>dur)t=0;show();},1000/fps);}
  else clearInterval(timer);
};
</script></body></html>`
