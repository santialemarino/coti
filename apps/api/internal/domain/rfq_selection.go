package domain

// RFQSelectionPreference is an explicit criterion for choosing a requested catalog product.
type RFQSelectionPreference string

const (
	RFQSelectionNone         RFQSelectionPreference = "NONE"
	RFQSelectionLowestPrice  RFQSelectionPreference = "LOWEST_PRICE"
	RFQSelectionHighestPrice RFQSelectionPreference = "HIGHEST_PRICE"
)

// RFQPriceCandidate is a branch product eligible for comparison with a matched product.
type RFQPriceCandidate struct {
	Product  Product
	Synonyms []string
}
