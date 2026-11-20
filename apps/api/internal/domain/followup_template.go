package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DefaultFollowupTemplate is the initial seller-editable reminder.
const DefaultFollowupTemplate = "¿Pudiste revisar la cotización {cotizacion}? Quedamos atentos a tu respuesta."

var followupParameter = regexp.MustCompile(`\{[^{}]*\}`)

// QuoteFollowupTemplate is a rendered reminder and its configuration state.
type QuoteFollowupTemplate struct {
	Message            string
	TemplateConfigured bool
}

// ValidateFollowupTemplate accepts the supported parameters and bounded nonempty text.
func ValidateFollowupTemplate(value string) error {
	if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 2000 {
		return ErrInvalidInput
	}
	for _, parameter := range followupParameter.FindAllString(value, -1) {
		if parameter != "{cliente}" && parameter != "{cotizacion}" {
			return fmt.Errorf("%w: unsupported follow-up parameter %s", ErrInvalidInput, parameter)
		}
	}
	return nil
}

// RenderFollowupTemplate substitutes customer and quote references without a model call.
func RenderFollowupTemplate(value, client string, number int64) string {
	return strings.NewReplacer("{cliente}", client, "{cotizacion}", QuoteReference(number)).Replace(value)
}
