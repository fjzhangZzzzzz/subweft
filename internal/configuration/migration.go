package configuration

import (
	"strings"
	"time"
)

func migrateDocument(d Document) (Document, *MigrationReport) {
	legacy := false
	for _, source := range d.Draft.Config.Subscriptions {
		legacy = legacy || strings.TrimSpace(source.Pool) != ""
	}
	for _, group := range d.Draft.Config.Groups {
		legacy = legacy || len(group.Pools) > 0
	}
	for _, release := range d.Releases {
		for _, source := range release.Config.Subscriptions {
			legacy = legacy || strings.TrimSpace(source.Pool) != ""
		}
		for _, group := range release.Config.Groups {
			legacy = legacy || len(group.Pools) > 0
		}
	}
	if !legacy {
		return d, nil
	}
	report := &MigrationReport{CompletedAt: time.Now().UTC()}
	d.Draft.Config = migrateConfig(d.Draft.Config, report)
	for i := range d.Releases {
		d.Releases[i].Config = migrateConfig(d.Releases[i].Config, &MigrationReport{})
	}
	if report.Subscriptions == 0 && report.Groups == 0 && report.UnknownMappings == 0 && report.QualityMappings == 0 && report.QuotaMappings == 0 {
		return d, nil
	}
	return d, report
}

func migrateConfig(c Config, report *MigrationReport) Config {
	c = Clone(c)
	for i := range c.Subscriptions {
		s := &c.Subscriptions[i]
		legacy := strings.TrimSpace(s.Pool)
		tags := append([]string{}, s.Tags...)
		switch legacy {
		case "quality":
			tags = append(tags, "高质量")
			report.QualityMappings++
		case "quota":
			tags = append(tags, "高流量")
			report.QuotaMappings++
		case "":
		case SystemUnclassifiedTag:
			// The old model had no system label; treat it as fallback.
			report.UnknownMappings++
		default:
			report.UnknownMappings++
			report.Warnings = append(report.Warnings, "订阅「"+s.Name+"」的旧分组值已归入未分类")
		}
		normalized := make([]string, 0, len(tags))
		for _, tag := range tags {
			value := strings.TrimSpace(tag)
			if value == "" || value == SystemUnclassifiedTag {
				continue
			}
			if len([]rune(value)) > maxTagLength || strings.ContainsAny(value, "\r\n,，") {
				report.UnknownMappings++
				report.Warnings = append(report.Warnings, "订阅「"+s.Name+"」存在无法识别的标签，已归入未分类")
				continue
			}
			normalized = append(normalized, value)
		}
		if deduped, err := NormalizeTags(normalized, false); err == nil {
			s.Tags = deduped
		} else {
			s.Tags = nil
			report.UnknownMappings++
			report.Warnings = append(report.Warnings, "订阅「"+s.Name+"」的标签超出限制，已归入未分类")
		}
		s.Pool = ""
		report.Subscriptions++
	}
	for i := range c.Groups {
		g := &c.Groups[i]
		tags := append([]string{}, g.Tags...)
		for _, legacy := range g.Pools {
			switch strings.TrimSpace(legacy) {
			case "quality":
				tags = append(tags, "高质量")
			case "quota":
				tags = append(tags, "高流量")
			case "", SystemUnclassifiedTag:
				if legacy == SystemUnclassifiedTag {
					tags = append(tags, SystemUnclassifiedTag)
				}
			default:
				report.UnknownMappings++
				report.Warnings = append(report.Warnings, "策略组「"+g.Name+"」的旧分组值已归入未分类")
			}
		}
		if len(tags) == 0 {
			tags = []string{SystemUnclassifiedTag}
		}
		if normalized, err := NormalizeTags(tags, true); err == nil {
			g.Tags = normalized
		} else {
			g.Tags = []string{SystemUnclassifiedTag}
			report.UnknownMappings++
			report.Warnings = append(report.Warnings, "策略组「"+g.Name+"」的标签无法识别，已归入未分类")
		}
		g.Pools = nil
		report.Groups++
	}
	return c
}
