package llmreg

import (
	"flag"
	"os"
	"testing"
)

var refresh = flag.Bool("refresh", false, "refresh the models.dev cache in testdata")

func TestMain(m *testing.M) {
	cacheDir = "testdata"
	os.Exit(m.Run())
}

func TestProviders(t *testing.T) {
	pvdrs, err := Providers(*refresh)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"anthropic", "openai", "google"} {
		pvdr, ok := pvdrs[id]
		if !ok {
			t.Fatalf("provider %s not found", id)
		}
		if pvdr.ID != id || len(pvdr.Models) == 0 {
			t.Fatalf("provider %s: id = %q, %d models", id, pvdr.ID, len(pvdr.Models))
		}
		for key, m := range pvdr.Models {
			if m.ID == "" || m.Name == "" {
				t.Errorf("%s: model %s: missing id or name: %+v", id, key, m)
			}
		}
	}
}

func TestFindProvider(t *testing.T) {
	pvdr, err := FindProvider("anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if pvdr.ID != "anthropic" || len(pvdr.Models) == 0 {
		t.Fatalf("anthropic: id = %q, %d models", pvdr.ID, len(pvdr.Models))
	}

	_, err = FindProvider("no-such-provider")
	if err == nil {
		t.Fatal("FindProvider(no-such-provider) did not fail")
	}
}
