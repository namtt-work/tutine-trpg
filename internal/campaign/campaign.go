package campaign

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Campaign struct {
	ID            string              `yaml:"id"`
	Name          string              `yaml:"name"`
	StartingScene string              `yaml:"starting_scene"`
	StartingRealm string              `yaml:"starting_realm"`
	StartingStage int                 `yaml:"starting_stage"`
	Tags          map[string][]string `yaml:"-"`
	tagSet        map[string]bool
}

func Load(dir string) (Campaign, error) {
	campaignData, err := os.ReadFile(filepath.Join(dir, "campaign.yaml"))
	if err != nil {
		return Campaign{}, err
	}
	var camp Campaign
	if err := yaml.Unmarshal(campaignData, &camp); err != nil {
		return Campaign{}, err
	}
	tagData, err := os.ReadFile(filepath.Join(dir, "tags.yaml"))
	if err != nil {
		return Campaign{}, err
	}
	var tags map[string][]string
	if err := yaml.Unmarshal(tagData, &tags); err != nil {
		return Campaign{}, err
	}
	camp.Tags = tags
	camp.tagSet = map[string]bool{}
	for _, values := range tags {
		for _, tag := range values {
			camp.tagSet[tag] = true
		}
	}
	return camp, nil
}

func (c Campaign) HasTag(tag string) bool {
	return c.tagSet[tag]
}
