package configuration

import (
	"context"
	"strings"
	"testing"
)

func TestImportPreservesRoutingAndRejectsUnsupportedInputs(t *testing.T) {
	yaml, err := Render(fixture(), false)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ImportPreview(string(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Subscriptions) != 1 || len(c.RuleSets) != 2 || len(c.Rules) != 2 || c.Rules[0].NoResolve || !c.Rules[1].NoResolve {
		t.Fatal("mixed providers must preserve domain then IP routing and no-resolve")
	}
	if _, err = Render(c, false); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		string(yaml) + "\n---\nrules: [MATCH,REJECT]\n",
		string(yaml) + "\nproxies: []\n",
		strings.Replace(string(yaml), "interval: 3600", "interval: wrong", 1),
		strings.Replace(string(yaml), "enable: false", "enable: wrong", 1),
		strings.Replace(string(yaml), "additional-prefix: \"\"", "additional-prefix: 123", 1),
		strings.Replace(string(yaml), "proxy: DIRECT", "proxy: true", 1),
		strings.Replace(string(yaml), "mixed-port: 7890", "mixed-port: wrong", 1),
	} {
		if _, err := ImportPreview(input); err == nil {
			t.Fatal("unsupported input must be rejected")
		}
	}
}

func TestInitialImportAndRestoreKeepSecretsOnServer(t *testing.T) {
	s, _, _ := setup(t, &checker{})
	ctx := context.Background()
	doc, _, _ := s.Read(ctx)
	yaml, _ := Render(fixture(), false)
	draft, err := s.Import(ctx, doc.Draft.Revision, string(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Import(ctx, draft.Revision, string(yaml)); err == nil {
		t.Fatal("second import accepted")
	}
	if _, err = s.Check(ctx, draft.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, draft.Revision, "initial"); err != nil {
		t.Fatal(err)
	}
	before := Clone(draft.Config)
	draft.Config.Subscriptions[0].URL = "https://replacement.example.com/subscription"
	draft, err = s.Save(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	oldYAML, _ := RenderDiff(before, draft.Config, "旧值")
	newYAML, _ := RenderDiff(draft.Config, before, "新值")
	if string(oldYAML) == string(newYAML) || strings.Contains(string(newYAML), "replacement.example.com") || strings.Contains(string(oldYAML), "subscription.example.com") {
		t.Fatal("secret diff must distinguish sides without URLs")
	}
	draft, err = s.Restore(ctx, draft.Revision, 1)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Config.Subscriptions[0].URL != before.Subscriptions[0].URL {
		t.Fatal("restore did not restore original secret")
	}
}
