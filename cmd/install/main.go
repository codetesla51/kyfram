// Command install is kyfram's interactive setup: it checks the host for
// everything kyfram needs (ffmpeg, edge-tts, go, git), offers to install
// what's missing through the system package manager or pip, then builds
// the kyfram binary and points at the scaffold command.
//
//	go run ./cmd/install [--yes] [--check] [--version vX.Y.Z]
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const repo = "https://gitlab.com/codetesla51/kyfram.git"

// paint wraps s in an ANSI code on terminals only.
func paint(f *os.File, code, s string) string {
	if os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(f.Fd())) {
		return code + s + "\x1b[0m"
	}
	return s
}

func ok(s string) string  { return paint(os.Stdout, "\x1b[32m", s) }
func bad(s string) string { return paint(os.Stderr, "\x1b[31m", s) }
func dim(s string) string { return paint(os.Stdout, "\x1b[2m", s) }

func main() {
	var yes, check bool
	var version string
	root := &cobra.Command{
		Use:   "install [--yes] [--check] [--version vX.Y.Z]",
		Short: "Interactive kyfram setup: check tools, install missing, build kyfram",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(yes, check, version)
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.Flags().BoolVarP(&yes, "yes", "y", false, "accept every install prompt")
	root.Flags().BoolVar(&check, "check", false, "report only, change nothing (exit 1 if anything missing)")
	root.Flags().StringVar(&version, "version", "latest", "kyfram tag to build (default: newest tag)")
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, bad("install: "+err.Error()))
		os.Exit(1)
	}
}

// tool is one host dependency: how to detect it, how to install it.
type tool struct {
	name    string
	why     string
	check   func() (found bool, detail string)
	install func(pm string) error // pm is "" when no package manager exists
}

func run(yes, checkOnly bool, version string) error {
	in := bufio.NewReader(os.Stdin)
	ask := func(q string) bool {
		if yes {
			fmt.Println(q, dim("[yes]"))
			return true
		}
		fmt.Print(q + " [Y/n] ")
		line, _ := in.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		return line == "" || line == "y" || line == "yes"
	}

	pm := detectPM()
	tools := []tool{
		{"ffmpeg", "encodes mp4 (png works without it)",
			func() (bool, string) { return look("ffmpeg", "--version", 0) },
			func(pm string) error { return pmInstall(pm, "ffmpeg") }},
		{"edge-tts", "neural voices (falls back to espeak-ng, then beeps)",
			func() (bool, string) { return look("edge-tts", "--version", 0) },
			func(_ string) error { return pipInstall("edge-tts") }},
		{"go", "builds kyfram (need 1.25+)",
			checkGo,
			func(_ string) error {
				return fmt.Errorf("install Go 1.25+ from https://go.dev/dl, then re-run")
			}},
		{"git", "fetches the kyfram source",
			func() (bool, string) { return look("git", "--version", 0) },
			func(pm string) error { return pmInstall(pm, "git") }},
	}

	fmt.Println("kyfram setup — checking host tools:")
	missing := 0
	for _, t := range tools {
		found, detail := t.check()
		if found {
			fmt.Printf("  %s %-9s %s\n", ok("found"), t.name, dim(detail))
			continue
		}
		missing++
		fmt.Printf("  %s %-9s %s\n", bad("missing"), t.name, dim(t.why))
	}
	if missing == 0 {
		fmt.Println(ok("everything present"))
	} else if checkOnly {
		return fmt.Errorf("%d tool(s) missing", missing)
	} else {
		fmt.Printf("\nkyfram needs %s. Install what's missing?\n", missingNoun(missing))
		if !ask("Proceed") {
			return fmt.Errorf("aborted — install the missing tools and re-run")
		}
		for _, t := range tools {
			if found, _ := t.check(); found {
				continue
			}
			if err := withSpinner("installing "+t.name, func() error { return t.install(pm) }); err != nil {
				return fmt.Errorf("%s: %w", t.name, err)
			}
			if found, detail := t.check(); found {
				fmt.Printf("  %s %-9s %s\n", ok("ok"), t.name, dim(detail))
			} else {
				return fmt.Errorf("%s installed but still not found: %s", t.name, detail)
			}
		}
	}

	// Build kyfram itself.
	if version == "latest" {
		var err error
		version, err = newestTag()
		if err != nil {
			return fmt.Errorf("resolve latest tag: %w", err)
		}
		fmt.Println("newest tag:", version)
	}
	bin, err := installKyfram(version)
	if err != nil {
		return err
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return fmt.Errorf("verify %s: %w", bin, err)
	}
	fmt.Printf("\n%s (%s)\n", ok(strings.TrimSpace(string(out))), bin)
	if dir := filepath.Dir(bin); !onPATH(dir) {
		fmt.Printf("add to PATH: %s\n", dim(fmt.Sprintf("export PATH=$PATH:%s", dir)))
	}
	fmt.Println("\nScaffold your first video:")
	fmt.Println("  kyfram new demo   ", dim("# demo/scene.lgv + assets/ + out/"))
	fmt.Println("  kyfram render --out demo/out/demo.mp4 demo/scene.lgv")
	return nil
}

func missingNoun(n int) string {
	if n == 1 {
		return "one tool"
	}
	return fmt.Sprintf("%d tools", n)
}

// look reports whether bin exists and runs rendezvous-free: `bin arg`
// must exit 0, first output line becomes the detail.
func look(bin, arg string, _ int) (bool, string) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return false, "not on PATH"
	}
	out, err := exec.Command(path, arg).Output()
	if err != nil {
		return true, path
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return true, strings.TrimSpace(line)
}

// checkGo requires Go 1.25+.
func checkGo() (bool, string) {
	found, detail := look("go", "version", 0)
	if !found {
		return false, detail
	}
	m := regexp.MustCompile(`go(\d+)\.(\d+)`).FindStringSubmatch(detail)
	if m == nil {
		return true, detail
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 1 || (major == 1 && minor < 25) {
		return false, detail + " (need 1.25+)"
	}
	return true, detail
}

// detectPM returns the first available system package manager, or "".
func detectPM() string {
	for _, pm := range []string{"pacman", "apt-get", "dnf", "apk", "zypper", "brew"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

// pmInstall installs pkg through pm, with sudo outside root.
func pmInstall(pm, pkg string) error {
	if pm == "" {
		return fmt.Errorf("no package manager found — install %s by hand", pkg)
	}
	args := []string{"-S", "--noconfirm", "--needed", pkg} // pacman shape
	switch pm {
	case "apt-get":
		args = []string{"install", "-y", pkg}
		if out, err := sudo("apt-get", "update"); err != nil {
			return fmt.Errorf("apt-get update: %v\n%s", err, out)
		}
	case "dnf":
		args = []string{"install", "-y", pkg}
	case "apk":
		args = []string{"add", pkg}
	case "zypper":
		args = []string{"--non-interactive", "install", pkg}
	case "brew":
		args = []string{"install", pkg}
	}
	out, err := sudo(pm, args...)
	if err != nil {
		return fmt.Errorf("%s\n%s", err, out)
	}
	return nil
}

// sudo prefixes with sudo outside root (and outside brew, which refuses it).
func sudo(bin string, args ...string) (string, error) {
	if os.Geteuid() != 0 && bin != "brew" {
		if _, err := exec.LookPath("sudo"); err != nil {
			return "", fmt.Errorf("need root for %s — re-run with sudo", bin)
		}
		args = append([]string{bin}, args...)
		bin = "sudo"
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

// pipInstall prefers pipx, falls back to user-site pip.
func pipInstall(pkg string) error {
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("python3 not found — install it first")
	}
	if _, err := exec.LookPath("pipx"); err == nil {
		out, err := exec.Command("pipx", "install", pkg).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s", out)
		}
		return nil
	}
	out, err := exec.Command("python3", "-m", "pip", "install", "--user", pkg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// newestTag resolves the highest v* tag on the remote.
func newestTag() (string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", repo).Output()
	if err != nil {
		return "", err
	}
	var tags []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !strings.HasPrefix(f[1], "refs/tags/v") || strings.HasSuffix(f[1], "^{}") {
			continue
		}
		tags = append(tags, strings.TrimPrefix(f[1], "refs/tags/"))
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("no v* tags found")
	}
	sort.Slice(tags, func(i, j int) bool { return semverLess(tags[i], tags[j]) })
	return tags[len(tags)-1], nil
}

func semverLess(a, b string) bool {
	pa, pb := semverParts(a), semverParts(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func semverParts(s string) [3]int {
	var p [3]int
	for i, f := range strings.SplitN(strings.TrimPrefix(s, "v"), ".", 3) {
		if i > 2 {
			break
		}
		p[i], _ = strconv.Atoi(f)
	}
	return p
}

// installKyfram clones tag shallow and builds into a writable bin dir.
func installKyfram(tag string) (string, error) {
	dir := "/usr/local/bin"
	if fi, err := os.Stat(dir); err != nil || fi == nil {
		dir = ""
	} else if f, err := os.OpenFile(filepath.Join(dir, ".w"), os.O_WRONLY|os.O_CREATE, 0o644); err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "bin")
	} else {
		f.Close()
		os.Remove(filepath.Join(dir, ".w"))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "kyfram-install")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(dir, "kyfram")
	err = withSpinner(fmt.Sprintf("building kyfram %s → %s", tag, bin), func() error {
		clone := []string{"clone", "--depth", "1"}
		if tag != "master" {
			clone = append(clone, "--branch", tag)
		}
		clone = append(clone, repo, tmp+"/src")
		if out, err := exec.Command("git", clone...).CombinedOutput(); err != nil {
			return fmt.Errorf("clone: %v\n%s", err, out)
		}
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/kyfram")
		cmd.Dir = tmp + "/src"
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build: %v\n%s", err, out)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return bin, nil
}

// withSpinner runs fn behind a braille spinner labeled label.
func withSpinner(label string, fn func() error) error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Prefix = label + " "
	s.Start()
	err := fn()
	s.Stop()
	return err
}

// onPATH reports whether dir is on PATH.
func onPATH(dir string) bool {
	for _, p := range strings.Split(os.Getenv("PATH"), ":") {
		if p == dir {
			return true
		}
	}
	return false
}
