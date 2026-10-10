package catalog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func sampleEntries() []Emoji {
	return []Emoji{
		{Code: ":bug:", Emoji: "🐛", Name: "bug", Description: "Fix a bug."},
		{Code: ":memo:", Emoji: "📝", Name: "memo", Description: "Add documentation."},
	}
}

func catalogJSON(t *testing.T, entries []Emoji) []byte {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCache(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitmojis.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEmbeddedClassification(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 75 || len(c.Types) != 6 || !c.Compatible("fix", ":bug:") || c.Compatible("docs", ":bug:") {
		t.Fatalf("unexpected embedded catalog: entries=%d, types=%v", len(c.Entries), c.Types)
	}
	for _, test := range []struct {
		name   string
		mutate func(Catalog)
	}{
		{"missing type", func(c Catalog) { delete(c.Types, "fix") }},
		{"extra type", func(c Catalog) { c.Types["other"] = []string{":bug:"} }},
		{"empty type", func(c Catalog) { c.Types["fix"] = nil }},
		{"unknown code", func(c Catalog) { c.Types["fix"] = []string{":unknown:"} }},
		{"duplicate code", func(c Catalog) { c.Types["fix"] = []string{":bug:", ":bug:"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(c)
			if err := c.validateTypes(); err == nil {
				t.Fatal("invalid classification accepted")
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	entries := sampleEntries()
	entries = append(entries, Emoji{Code: ":new_item:", Emoji: "🌟", Name: "new-item", Description: "A new entry."})
	data := catalogJSON(t, entries)
	for _, document := range [][]byte{data, append(append([]byte(`{"gitmojis":`), data...), '}')} {
		c, err := ReadFile(writeCache(t, document))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.Entries, entries) || len(c.Types) != 6 || !c.Compatible("fix", ":bug:") || c.Compatible("feat", ":sparkles:") {
			t.Fatalf("unexpected cache catalog: %+v", c)
		}
		for kind := range c.Types {
			if c.Compatible(kind, ":new_item:") {
				t.Fatal("new entry received an arbitrary classification")
			}
		}
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestReadFileRejectsInvalidCatalog(t *testing.T) {
	for _, data := range []string{"", "null", "[]", "{}", `{"gitmojis":[]}`, `{"gitmojis":{}}`, "[", "[] []", strings.Repeat(" ", maxCatalogBytes+1), string([]byte{'[', 0xff, ']'})} {
		if _, err := ReadFile(writeCache(t, []byte(data))); err == nil {
			t.Fatalf("invalid catalog accepted (length %d)", len(data))
		}
	}
	for _, test := range []struct {
		name   string
		mutate func([]Emoji)
	}{
		{"missing code", func(e []Emoji) { e[0].Code = "" }},
		{"missing emoji", func(e []Emoji) { e[0].Emoji = " " }},
		{"missing name", func(e []Emoji) { e[0].Name = "" }},
		{"missing description", func(e []Emoji) { e[0].Description = "\t" }},
		{"control emoji", func(e []Emoji) { e[0].Emoji = "🐛\x1b[0m" }},
		{"control description", func(e []Emoji) { e[0].Description = "Fix\na bug" }},
		{"invalid code", func(e []Emoji) { e[0].Code = "bug" }},
		{"invalid code syntax", func(e []Emoji) { e[0].Code = ":bad code:" }},
		{"invalid name syntax", func(e []Emoji) { e[0].Name = "Bad name" }},
		{"duplicate code", func(e []Emoji) { e[1].Code = e[0].Code }},
		{"duplicate name", func(e []Emoji) { e[1].Name = e[0].Name }},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := sampleEntries()
			test.mutate(entries)
			if _, err := ReadFile(writeCache(t, catalogJSON(t, entries))); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestSearchRanking(t *testing.T) {
	c := Catalog{Entries: []Emoji{
		{Code: ":substring_description:", Name: "find", Description: "Find a bug here"},
		{Code: ":substring_name:", Name: "debug", Description: "Inspect code"},
		{Code: ":prefix_description:", Name: "fix", Description: "Bug fixes"},
		{Code: ":prefix_name:", Name: "bug-linter", Description: "Lint code"},
		{Code: ":exact_description:", Name: "other", Description: "bug"},
		{Code: ":exact_name:", Name: "bug", Description: "Fix errors"},
		{Code: ":tokens:", Name: "linter", Description: "Find a bug"},
	}}
	var codes []string
	for _, entry := range c.Search(" BuG ") {
		codes = append(codes, entry.Code)
	}
	want := []string{":exact_description:", ":exact_name:", ":prefix_name:", ":prefix_description:", ":substring_name:", ":substring_description:", ":tokens:"}
	if !slices.Equal(codes, want) {
		t.Fatalf("ranking=%v, want %v", codes, want)
	}
	if result := c.Search("bug linter"); len(result) != 2 || result[0].Code != ":prefix_name:" || result[1].Code != ":tokens:" {
		t.Fatalf("token matches: %v", result)
	}
	if len(c.Search(":exact_name:")) != 0 || len(c.Search("nonexistent")) != 0 || !reflect.DeepEqual(c.Search(""), c.Entries) {
		t.Fatal("search scope or empty query behavior is incorrect")
	}
	if c.Entries[0].Code != ":substring_description:" {
		t.Fatal("search mutated catalog order")
	}
	embedded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := embedded.Search("bug"); len(got) == 0 || got[0].Code != ":bug:" {
		t.Fatalf("representative bug search: %v", got)
	}
	if got := embedded.Search("linter"); len(got) != 1 || got[0].Code != ":rotating_light:" {
		t.Fatalf("representative linter search: %v", got)
	}
}

func TestCachePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path, err := CachePath()
	if err != nil || path != filepath.Join(home, ".gitmoji", "gitmojis.json") {
		t.Fatalf("cache path=%q, error=%v", path, err)
	}
	t.Setenv("HOME", "relative-home")
	t.Setenv("USERPROFILE", "relative-home")
	if path, err := CachePath(); err == nil || path != "" {
		t.Fatalf("relative home accepted: path=%q, error=%v", path, err)
	}
}

func TestUpdateRepairsUnchangedCache(t *testing.T) {
	current, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	document := `{"gitmojis":` + string(catalogJSON(t, current.Entries)) + `}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, document)
	}))
	defer server.Close()
	for _, corrupt := range []bool{false, true} {
		cache := filepath.Join(t.TempDir(), "gitmojis.json")
		if corrupt {
			cache = writeCache(t, []byte("invalid cache"))
		}
		changed, err := Update(context.Background(), server.Client(), server.URL, cache, current)
		if err != nil || changed {
			t.Fatalf("unchanged update changed=%v, error=%v", changed, err)
		}
		repaired, err := ReadFile(cache)
		if err != nil || !reflect.DeepEqual(repaired.Entries, current.Entries) {
			t.Fatalf("cache not initialized or repaired: %v", err)
		}
		data, err := os.ReadFile(cache)
		if err != nil || len(data) == 0 || data[0] != '[' {
			t.Fatalf("repaired cache is not a legacy array: %v", err)
		}
	}
}

func TestUpdateChangesAndReordering(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  func([]Emoji) []Emoji
		changed bool
	}{
		{"reorder", func(e []Emoji) []Emoji { slices.Reverse(e); return e }, false},
		{"description", func(e []Emoji) []Emoji { e[0].Description = "Fix more bugs."; return e }, true},
		{"emoji", func(e []Emoji) []Emoji { e[0].Emoji = "🐞"; return e }, true},
		{"name", func(e []Emoji) []Emoji { e[0].Name = "insect"; return e }, true},
		{"new unclassified", func(e []Emoji) []Emoji { e[0].Code = ":new_item:"; return e }, true},
		{"added entry", func(e []Emoji) []Emoji {
			return append(e, Emoji{Code: ":new_item:", Emoji: "🌟", Name: "new-item", Description: "A new entry."})
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := catalogJSON(t, sampleEntries())
			cache := writeCache(t, original)
			latest := test.mutate(sampleEntries())
			document := `{"gitmojis":` + string(catalogJSON(t, latest)) + `}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, document)
			}))
			defer server.Close()
			changed, err := Update(context.Background(), server.Client(), server.URL, cache, Catalog{Entries: sampleEntries()})
			if err != nil || changed != test.changed {
				t.Fatalf("changed=%v, error=%v", changed, err)
			}
			data, err := os.ReadFile(cache)
			if err != nil {
				t.Fatal(err)
			}
			if !test.changed {
				if string(data) != string(original) {
					t.Fatal("reorder replaced the cache")
				}
				return
			}
			if len(data) == 0 || data[0] != '[' {
				t.Fatal("cache is not a legacy array")
			}
			c, err := ReadFile(cache)
			if err != nil || !reflect.DeepEqual(c.Entries, latest) {
				t.Fatalf("updated cache=%+v, error=%v", c, err)
			}
			for kind := range c.Types {
				if c.Compatible(kind, ":new_item:") {
					t.Fatal("new entry was classified")
				}
			}
		})
	}
}

func TestUpdateFailuresPreserveCache(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP error", http.StatusServiceUnavailable, "unavailable"},
		{"unexpected success status", http.StatusNoContent, ""},
		{"invalid JSON", http.StatusOK, "invalid"},
		{"empty catalog", http.StatusOK, `{"gitmojis":[]}`},
		{"invalid entry", http.StatusOK, `[{"code":":bug:","emoji":"🐛","name":"bug"}]`},
		{"oversized", http.StatusOK, strings.Repeat(" ", maxCatalogBytes+1)},
		{"invalid UTF-8", http.StatusOK, string([]byte{0xff})},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := catalogJSON(t, sampleEntries())
			cache := writeCache(t, original)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			changed, err := Update(context.Background(), server.Client(), server.URL, cache, Catalog{Entries: sampleEntries()})
			if err == nil || changed {
				t.Fatalf("changed=%v, error=%v", changed, err)
			}
			data, err := os.ReadFile(cache)
			if err != nil || string(data) != string(original) {
				t.Fatalf("cache lost after failure: %v", err)
			}
		})
	}
}

func TestUpdateTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	for _, useContext := range []bool{false, true} {
		original := catalogJSON(t, sampleEntries())
		cache := writeCache(t, original)
		ctx := context.Background()
		client := *server.Client()
		if useContext {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
			defer cancel()
		} else {
			client.Timeout = 30 * time.Millisecond
		}
		changed, err := Update(ctx, &client, server.URL, cache, Catalog{Entries: sampleEntries()})
		if err == nil || changed {
			t.Fatalf("timeout changed=%v, error=%v", changed, err)
		}
		data, err := os.ReadFile(cache)
		if err != nil || string(data) != string(original) {
			t.Fatalf("cache lost after timeout: %v", err)
		}
	}
}

func TestUpdateInvalidURLAndWriteFailure(t *testing.T) {
	cache := writeCache(t, catalogJSON(t, sampleEntries()))
	for _, url := range []string{"file:///tmp/catalog", "http://", ":bad"} {
		if changed, err := Update(context.Background(), nil, url, cache, Catalog{}); err == nil || changed {
			t.Fatalf("invalid URL %q accepted: %v", url, err)
		}
	}
	document := catalogJSON(t, sampleEntries())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(document)
	}))
	defer server.Close()
	if changed, err := Update(context.Background(), nil, server.URL, t.TempDir(), Catalog{}); err == nil || changed {
		t.Fatalf("write failure changed=%v, error=%v", changed, err)
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport.Proxy == nil || reflect.ValueOf(transport.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
		t.Fatal("default transport no longer uses environment proxy handling")
	}
}

func TestEnvironmentProxy(t *testing.T) {
	if url := os.Getenv("CATALOG_PROXY_TEST_URL"); url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		proxy, err := http.DefaultTransport.(*http.Transport).Proxy(req)
		got := ""
		if proxy != nil {
			got = proxy.String()
		}
		if err != nil || got != os.Getenv("CATALOG_PROXY_TEST_WANT") {
			t.Fatalf("proxy=%q, error=%v", got, err)
		}
		return
	}
	// net/http caches environment proxies; use a fresh test process per case.
	for _, test := range []struct {
		url, want string
		env       []string
	}{
		{"http://catalog.example.test", "http://proxy.example:8080", []string{"HTTP_PROXY=http://proxy.example:8080"}},
		{"https://catalog.example.test", "http://secure-proxy.example:8080", []string{"HTTPS_PROXY=http://secure-proxy.example:8080"}},
		{"https://catalog.example.test", "", []string{"HTTPS_PROXY=http://proxy.example:8080", "NO_PROXY=catalog.example.test"}},
		{"http://catalog.example.test", "http://proxy.example:8080", []string{"http_proxy=http://proxy.example:8080"}},
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEnvironmentProxy$")
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			switch strings.ToUpper(key) {
			case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "REQUEST_METHOD", "CATALOG_PROXY_TEST_URL", "CATALOG_PROXY_TEST_WANT":
				continue
			}
			cmd.Env = append(cmd.Env, value)
		}
		cmd.Env = append(cmd.Env, test.env...)
		cmd.Env = append(cmd.Env, "CATALOG_PROXY_TEST_URL="+test.url, "CATALOG_PROXY_TEST_WANT="+test.want)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("proxy check: %v\n%s", err, output)
		}
	}
}
