package configuration

import "testing"

func TestTagsMatchUsesFallbackAndAnyTag(t *testing.T) {
	untagged := Subscription{Enabled: true}
	if !tagsMatch(untagged, []string{SystemUnclassifiedTag}) {
		t.Fatal("untagged subscription should match the system fallback tag")
	}
	tagged := Subscription{Tags: []string{"高质量", "海外"}}
	if !tagsMatch(tagged, []string{"不存在", "海外"}) {
		t.Fatal("group tags should use OR semantics")
	}
}

func TestNormalizeTagsTrimsAndDeduplicatesCaseInsensitive(t *testing.T) {
	got, err := NormalizeTags([]string{" 海外 ", "Overseas", "overseas"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "海外" || got[1] != "Overseas" {
		t.Fatalf("unexpected normalized tags: %#v", got)
	}
	if _, err = NormalizeTags([]string{SystemUnclassifiedTag}, false); err == nil {
		t.Fatal("subscriptions must not persist the system fallback tag")
	}
}

func TestMigrateDocumentMapsLegacyPoolsAndUnknowns(t *testing.T) {
	d := Document{
		Draft: Draft{Config: Config{
			Subscriptions: []Subscription{{ID: "s", Name: "S", Pool: "quality", URL: "https://example.com/s", Enabled: true, Interval: 3600}},
			Groups:        []Group{{ID: "g", Name: "G", Type: "select", Pools: []string{"mystery"}}},
		}},
	}
	got, report := migrateDocument(d)
	if got.Draft.Config.Subscriptions[0].Tags[0] != "高质量" {
		t.Fatalf("quality pool was not migrated: %#v", got.Draft.Config.Subscriptions[0].Tags)
	}
	if len(got.Draft.Config.Groups[0].Tags) != 1 || got.Draft.Config.Groups[0].Tags[0] != SystemUnclassifiedTag {
		t.Fatalf("unknown group pool was not moved to fallback: %#v", got.Draft.Config.Groups[0].Tags)
	}
	if report == nil || report.UnknownMappings == 0 {
		t.Fatal("migration warning for unknown value missing")
	}
}
