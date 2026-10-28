package domain

// Empty selections have one wire representation, including migrated/older records.
func NormalizeProfile(p *Profile) {
	if p.Groups == nil {
		p.Groups = []Group{}
	}
	if p.Rules == nil {
		p.Rules = []Rule{}
	}
	if p.Settings.Nameservers == nil {
		p.Settings.Nameservers = []string{}
	}
	for i := range p.Groups {
		g := &p.Groups[i]
		if g.SourceIDs == nil {
			g.SourceIDs = []string{}
		}
		if g.Tags == nil {
			g.Tags = []string{}
		}
		if g.NodeIDs == nil {
			g.NodeIDs = []string{}
		}
		if g.Groups == nil {
			g.Groups = []string{}
		}
		if g.Builtins == nil {
			g.Builtins = []string{}
		}
	}
}
