package manager

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OnyxNeol/eaglercmp/config"
)

// JavaClient prepares and runs a real desktop Minecraft client with Fabric
// Loader in a local JVM, using an OFFLINE session (custom username, no
// Microsoft login) so it can join the local offline-mode Velocity/Fabric
// stack. Layout under Dir: game/ (gameDir; put Fabric client .jar mods in
// game/mods), libraries/, assets/, versions/<mc>/.
//
// Mods are NOT put on the JVM classpath by hand: Fabric Loader's Knot class
// loader discovers every jar in game/mods itself and remaps/loads it, which
// is the only way unmodified Fabric mods work. Fabric API is downloaded from
// Modrinth into game/mods automatically.
type JavaClient struct {
	Cfg    config.JavaClient
	Root   string
	Server string // default host:port when Cfg.Server is empty
	Log    *Logger
	HTTP   *http.Client
}

const (
	mojangManifest = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	fabricMeta     = "https://meta.fabricmc.net/v2/versions/loader/"
	modrinthAPI    = "https://api.modrinth.com/v2/"
	defaultMC      = "26.2"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

func (j *JavaClient) version() string  { return orDefault(j.Cfg.Version, defaultMC) }
func (j *JavaClient) username() string { return orDefault(j.Cfg.Username, "Player") }
func (j *JavaClient) dir() string {
	return orDefault(j.Cfg.Dir, filepath.Join(j.Root, "javaclient"))
}

// GameDir is the Minecraft game directory; drop client mods in GameDir()/mods.
func (j *JavaClient) GameDir() string { return filepath.Join(j.dir(), "game") }

func orDefault(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

// OfflineUUID is the vanilla offline-mode UUID for a name (what an offline
// server derives), so the player keeps the same identity on every launch.
func OfflineUUID(name string) string {
	h := md5.Sum([]byte("OfflinePlayer:" + name))
	h[6] = h[6]&0x0f | 0x30
	h[8] = h[8]&0x3f | 0x80
	x := hex.EncodeToString(h[:])
	return x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
}

// ---- metadata types -------------------------------------------------------

type jcArtifact struct {
	Path string `json:"path"`
	SHA1 string `json:"sha1"`
	URL  string `json:"url"`
}
type jcRule struct {
	Action string `json:"action"`
	OS     *struct {
		Name string `json:"name"`
		Arch string `json:"arch"`
	} `json:"os"`
	Features map[string]bool `json:"features"`
}
type jcLib struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"` // Fabric-style maven base
	SHA1      string   `json:"sha1"`
	Rules     []jcRule `json:"rules"`
	Downloads struct {
		Artifact *jcArtifact `json:"artifact"`
	} `json:"downloads"`
}
type jcArg struct {
	Value []string
	Rules []jcRule
}

func (a *jcArg) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Value = []string{s}
		return nil
	}
	var o struct {
		Rules []jcRule        `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	a.Rules = o.Rules
	if json.Unmarshal(o.Value, &s) == nil {
		a.Value = []string{s}
		return nil
	}
	return json.Unmarshal(o.Value, &a.Value)
}

type jcVersion struct {
	ID        string `json:"id"`
	MainClass string `json:"mainClass"`
	Arguments struct {
		Game []jcArg `json:"game"`
		JVM  []jcArg `json:"jvm"`
	} `json:"arguments"`
	Libraries  []jcLib `json:"libraries"`
	AssetIndex struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		SHA1 string `json:"sha1"`
	} `json:"assetIndex"`
	Assets    string `json:"assets"`
	Downloads struct {
		Client jcArtifact `json:"client"`
	} `json:"downloads"`
	JavaVersion struct {
		Major int `json:"majorVersion"`
	} `json:"javaVersion"`
	Type string `json:"type"`
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	}
	return "linux"
}

func archMatches(re string) bool {
	if re == "" {
		return true
	}
	ok, _ := regexp.MatchString(re, map[string]string{"amd64": "x86_64", "386": "x86", "arm64": "arm64"}[runtime.GOARCH])
	return ok
}

// allowed evaluates Mojang rules; rules gated on launcher features (demo,
// custom resolution, quick play) are treated as not matching.
func allowed(rules []jcRule) bool {
	if len(rules) == 0 {
		return true
	}
	res := false
	for _, r := range rules {
		match := len(r.Features) == 0
		if r.OS != nil {
			match = (r.OS.Name == "" || r.OS.Name == osName()) && archMatches(r.OS.Arch)
		}
		if match {
			res = r.Action == "allow"
		}
	}
	return res
}

// ---- HTTP helpers -----------------------------------------------------------

func (j *JavaClient) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "EaglerCMP/java-client (local launcher)")
	resp, err := j.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return resp, nil
}

func (j *JavaClient) getJSON(ctx context.Context, u string, v any) error {
	r, err := j.get(ctx, u)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(v)
}

// fetch downloads u to dest (atomically) unless it already exists with the
// expected hash; algo is "sha1" or "sha512"; an empty sum skips verification.
func (j *JavaClient) fetch(ctx context.Context, u, dest, algo, sum string) error {
	if sum != "" && fileSum(dest, algo) == sum {
		return nil
	}
	if sum == "" {
		if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var err error
	for try := 0; try < 3; try++ {
		if err = j.fetchOnce(ctx, u, dest, algo, sum); err == nil || ctx.Err() != nil {
			return err
		}
		time.Sleep(time.Duration(try+1) * time.Second)
	}
	return err
}

func newHash(algo string) hash.Hash {
	if algo == "sha512" {
		return sha512.New()
	}
	return sha1.New()
}

func fileSum(path, algo string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := newHash(algo)
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func (j *JavaClient) fetchOnce(ctx context.Context, u, dest, algo, sum string) error {
	r, err := j.get(ctx, u)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := newHash(algo)
	_, err = io.Copy(io.MultiWriter(f, h), r.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && sum != "" && hex.EncodeToString(h.Sum(nil)) != sum {
		err = fmt.Errorf("%s: checksum mismatch", u)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

type dl struct{ url, dest, algo, sum string }

// fetchAll downloads in parallel and returns the first error.
func (j *JavaClient) fetchAll(ctx context.Context, items []dl) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg   sync.WaitGroup
		once sync.Once
		fail error
		sem  = make(chan struct{}, 16)
	)
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it dl) {
			defer func() { <-sem; wg.Done() }()
			if err := j.fetch(ctx, it.url, it.dest, it.algo, it.sum); err != nil {
				once.Do(func() { fail = err; cancel() })
			}
		}(it)
	}
	wg.Wait()
	return fail
}

// mavenPath converts "group:artifact:version[:classifier]" to a repository path.
func mavenPath(name string) (string, error) {
	p := strings.Split(name, ":")
	if len(p) < 3 {
		return "", fmt.Errorf("bad library name %q", name)
	}
	file := p[1] + "-" + p[2]
	if len(p) > 3 {
		file += "-" + p[3]
	}
	return strings.ReplaceAll(p[0], ".", "/") + "/" + p[1] + "/" + p[2] + "/" + file + ".jar", nil
}

// ---- prepare ------------------------------------------------------------------

// plan is everything needed to start the JVM.
type jcPlan struct {
	mc        jcVersion
	fabric    jcVersion
	classpath []string
	items     []dl
}

func (j *JavaClient) resolve(ctx context.Context) (*jcPlan, error) {
	var man struct {
		Versions []struct{ ID, URL string } `json:"versions"`
	}
	if err := j.getJSON(ctx, mojangManifest, &man); err != nil {
		return nil, fmt.Errorf("Mojang version manifest: %w", err)
	}
	var vurl string
	for _, v := range man.Versions {
		if v.ID == j.version() {
			vurl = v.URL
		}
	}
	if vurl == "" {
		return nil, fmt.Errorf("Minecraft %s is not in Mojang's version manifest", j.version())
	}
	p := &jcPlan{}
	if err := j.getJSON(ctx, vurl, &p.mc); err != nil {
		return nil, err
	}
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := j.getJSON(ctx, fabricMeta+url.PathEscape(j.version()), &loaders); err != nil {
		return nil, fmt.Errorf("Fabric meta: %w", err)
	}
	lv := ""
	for _, l := range loaders {
		if l.Loader.Stable {
			lv = l.Loader.Version
			break
		}
	}
	if lv == "" {
		return nil, fmt.Errorf("Fabric Loader has no stable build for Minecraft %s", j.version())
	}
	if err := j.getJSON(ctx, fabricMeta+url.PathEscape(j.version())+"/"+lv+"/profile/json", &p.fabric); err != nil {
		return nil, fmt.Errorf("Fabric profile: %w", err)
	}

	d := j.dir()
	seen := map[string]bool{}
	add := func(l jcLib, fabric bool) error {
		if !allowed(l.Rules) {
			return nil
		}
		var it dl
		if a := l.Downloads.Artifact; a != nil && a.URL != "" {
			it = dl{a.URL, filepath.Join(d, "libraries", filepath.FromSlash(a.Path)), "sha1", a.SHA1}
		} else {
			rel, err := mavenPath(l.Name)
			if err != nil {
				return err
			}
			base := orDefault(l.URL, "https://libraries.minecraft.net/")
			it = dl{strings.TrimSuffix(base, "/") + "/" + rel, filepath.Join(d, "libraries", filepath.FromSlash(rel)), "sha1", l.SHA1}
		}
		if seen[it.dest] {
			return nil
		}
		seen[it.dest] = true
		p.items = append(p.items, it)
		p.classpath = append(p.classpath, it.dest)
		return nil
	}
	// Fabric libraries first so its ASM/mixin versions win over vanilla's.
	for _, l := range p.fabric.Libraries {
		if err := add(l, true); err != nil {
			return nil, err
		}
	}
	for _, l := range p.mc.Libraries {
		if err := add(l, false); err != nil {
			return nil, err
		}
	}
	jar := filepath.Join(d, "versions", j.version(), j.version()+".jar")
	p.items = append(p.items, dl{p.mc.Downloads.Client.URL, jar, "sha1", p.mc.Downloads.Client.SHA1})
	p.classpath = append(p.classpath, jar)
	return p, nil
}

// Prepare downloads the client, libraries, assets and Fabric API (idempotent).
func (j *JavaClient) Prepare(ctx context.Context) (*jcPlan, error) {
	j.Log.Printf("java-client: resolving Minecraft %s + Fabric Loader", j.version())
	p, err := j.resolve(ctx)
	if err != nil {
		return nil, err
	}
	j.Log.Printf("java-client: downloading %d libraries/jars", len(p.items))
	if err := j.fetchAll(ctx, p.items); err != nil {
		return nil, err
	}
	if err := j.fetchAssets(ctx, p); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(j.GameDir(), "mods"), 0o755); err != nil {
		return nil, err
	}
	if err := j.installFabricAPI(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (j *JavaClient) fetchAssets(ctx context.Context, p *jcPlan) error {
	root := filepath.Join(j.dir(), "assets")
	idx := filepath.Join(root, "indexes", p.mc.AssetIndex.ID+".json")
	if err := j.fetch(ctx, p.mc.AssetIndex.URL, idx, "sha1", p.mc.AssetIndex.SHA1); err != nil {
		return err
	}
	b, err := os.ReadFile(idx)
	if err != nil {
		return err
	}
	var ai struct {
		Objects map[string]struct{ Hash string } `json:"objects"`
	}
	if err := json.Unmarshal(b, &ai); err != nil {
		return err
	}
	var items []dl
	for _, o := range ai.Objects {
		items = append(items, dl{"https://resources.download.minecraft.net/" + o.Hash[:2] + "/" + o.Hash, filepath.Join(root, "objects", o.Hash[:2], o.Hash), "sha1", o.Hash})
	}
	j.Log.Printf("java-client: checking %d asset files", len(items))
	return j.fetchAll(ctx, items)
}

// installFabricAPI keeps the newest Modrinth Fabric API build for this
// Minecraft version in game/mods (older copies it installed are replaced).
func (j *JavaClient) installFabricAPI(ctx context.Context) error {
	q := modrinthAPI + "project/fabric-api/version?loaders=" + url.QueryEscape(`["fabric"]`) + "&game_versions=" + url.QueryEscape(`["`+j.version()+`"]`)
	var vs []struct {
		Files []struct {
			URL      string            `json:"url"`
			Filename string            `json:"filename"`
			Primary  bool              `json:"primary"`
			Hashes   map[string]string `json:"hashes"`
		} `json:"files"`
	}
	if err := j.getJSON(ctx, q, &vs); err != nil {
		return fmt.Errorf("Fabric API lookup: %w", err)
	}
	if len(vs) == 0 || len(vs[0].Files) == 0 {
		return fmt.Errorf("no Fabric API build on Modrinth for Minecraft %s yet", j.version())
	}
	f := vs[0].Files[0]
	for _, c := range vs[0].Files {
		if c.Primary {
			f = c
		}
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._+-]+\.jar$`).MatchString(f.Filename) {
		return fmt.Errorf("unexpected Fabric API file name %q", f.Filename)
	}
	mods := filepath.Join(j.GameDir(), "mods")
	if old, _ := filepath.Glob(filepath.Join(mods, "fabric-api-*.jar")); len(old) > 0 {
		for _, o := range old {
			if filepath.Base(o) != f.Filename {
				os.Remove(o)
			}
		}
	}
	return j.fetch(ctx, f.URL, filepath.Join(mods, f.Filename), "sha512", f.Hashes["sha512"])
}

// ---- command line ----------------------------------------------------------------

func (j *JavaClient) java() string { return orDefault(j.Cfg.Java, "java") }

// CheckJava verifies a runnable Java of at least the major version Mojang requires.
func (j *JavaClient) CheckJava(major int) error {
	bin, err := exec.LookPath(j.java())
	if err != nil {
		return fmt.Errorf("Java not found (%q): install JDK %d+ or set javaClient.java", j.java(), major)
	}
	out, err := exec.Command(bin, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Java at %s is not runnable: %v", bin, err)
	}
	m := regexp.MustCompile(`version "(\d+)`).FindSubmatch(out)
	if m == nil {
		return nil
	}
	if have, _ := strconv.Atoi(string(m[1])); have < major {
		return fmt.Errorf("Minecraft %s needs Java %d+, found Java %d (%s)", j.version(), major, have, bin)
	}
	return nil
}

func (j *JavaClient) ServerAddr() string { return orDefault(j.Cfg.Server, j.Server) }

// Command builds the JVM command line (Fabric's KnotClient main class) with an
// offline session: the access token is a dummy and user type is "legacy", so
// no Microsoft/Mojang authentication happens and the name is fully custom.
func (j *JavaClient) Command(p *jcPlan) (string, []string, error) {
	name := j.username()
	if !usernameRe.MatchString(name) {
		return "", nil, fmt.Errorf("username %q must be 3-16 letters, digits or underscores", name)
	}
	d := j.dir()
	sep := string(os.PathListSeparator)
	vars := map[string]string{
		"auth_player_name": name, "auth_uuid": OfflineUUID(name), "auth_access_token": "0",
		"auth_xuid": "0", "clientid": "0", "user_type": "legacy",
		"version_name": j.version(), "version_type": p.mc.Type,
		"game_directory": j.GameDir(), "assets_root": filepath.Join(d, "assets"),
		"assets_index_name": p.mc.AssetIndex.ID, "natives_directory": filepath.Join(d, "natives"),
		"launcher_name": "EaglerCMP", "launcher_version": "1",
		"classpath": strings.Join(p.classpath, sep), "classpath_separator": sep,
		"library_directory": filepath.Join(d, "libraries"),
	}
	expand := func(args []jcArg) []string {
		var out []string
		for _, a := range args {
			if !allowed(a.Rules) {
				continue
			}
			for _, v := range a.Value {
				out = append(out, regexp.MustCompile(`\$\{(\w+)\}`).ReplaceAllStringFunc(v, func(m string) string { return vars[m[2:len(m)-1]] }))
			}
		}
		return out
	}
	os.MkdirAll(filepath.Join(d, "natives"), 0o755)
	var args []string
	args = append(args, j.Cfg.JVMArgs...)
	args = append(args, expand(p.mc.Arguments.JVM)...)
	args = append(args, expand(p.fabric.Arguments.JVM)...)
	args = append(args, p.fabric.MainClass)
	args = append(args, expand(p.mc.Arguments.Game)...)
	args = append(args, expand(p.fabric.Arguments.Game)...)
	if s := j.ServerAddr(); s != "" {
		args = append(args, "--quickPlayMultiplayer", s)
	}
	return j.java(), args, nil
}

// Run prepares everything, then runs the client until it exits.
func (j *JavaClient) Run(ctx context.Context, dry bool, out io.Writer) (int, error) {
	if j.HTTP == nil {
		j.HTTP = &http.Client{Timeout: 15 * time.Minute}
	}
	p, err := j.Prepare(ctx)
	if err != nil {
		return 1, err
	}
	if err := j.CheckJava(p.mc.JavaVersion.Major); err != nil {
		return 1, err
	}
	bin, args, err := j.Command(p)
	if err != nil {
		return 1, err
	}
	if dry {
		// the token is a dummy, but keep the classpath out of the output
		fmt.Fprintf(out, "%s %s\n", bin, strings.Join(redactClasspath(args), " "))
		return 0, nil
	}
	j.Log.Printf("java-client: starting Minecraft %s as %q (offline) -> %s; mods in %s", j.version(), j.username(), j.ServerAddr(), filepath.Join(j.GameDir(), "mods"))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = j.GameDir()
	cmd.Stdout, cmd.Stderr = out, out
	hideWindow(cmd)
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 0, err
}

func redactClasspath(args []string) []string {
	out := append([]string{}, args...)
	for i := 1; i < len(out); i++ {
		if out[i-1] == "-cp" || out[i-1] == "-classpath" {
			out[i] = "<classpath>"
		}
	}
	return out
}
