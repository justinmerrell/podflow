package nav

import (
	"github.com/manifoldco/promptui"
)

type Option struct {
	Name  string
	Value string
}

func SelectPrompt(label string, options []Option) (string, error) {
	promptTemplates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   ` {{ "●" | cyan }} {{ .Name | cyan }}`,
		Inactive: `   {{ .Name | white }}`,
		Selected: `   {{ .Name | white }}`,
	}

	selector := promptui.Select{
		Label:     label,
		Items:     options,
		Templates: promptTemplates,
	}

	i, _, err := selector.Run()
	if err != nil {
		return "", err
	}
	return options[i].Value, nil
}
