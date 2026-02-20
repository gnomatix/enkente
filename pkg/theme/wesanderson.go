// Package theme provides color palettes for the enkente TUI.
// Palettes sourced from https://github.com/karthik/wesanderson
package theme

import "github.com/charmbracelet/lipgloss"

// Palette represents a named color palette.
type Palette struct {
	Name   string
	Colors []lipgloss.Color
}

// WesAndersonPalettes contains all available Wes Anderson palettes.
var WesAndersonPalettes = []Palette{
	{Name: "BottleRocket1", Colors: []lipgloss.Color{"#A42820", "#5F5647", "#9B110E", "#3F5151", "#4E2A1E", "#550307", "#0C1707"}},
	{Name: "BottleRocket2", Colors: []lipgloss.Color{"#FAD510", "#CB2314", "#273046", "#354823", "#1E1E1E"}},
	{Name: "Rushmore1", Colors: []lipgloss.Color{"#E1BD6D", "#EABE94", "#0B775E", "#35274A", "#F2300F"}},
	{Name: "Royal1", Colors: []lipgloss.Color{"#899DA4", "#C93312", "#FAEFD1", "#DC863B"}},
	{Name: "Royal2", Colors: []lipgloss.Color{"#9A8822", "#F5CDB4", "#F8AFA8", "#FDDDA0", "#74A089"}},
	{Name: "Zissou1", Colors: []lipgloss.Color{"#3B9AB2", "#78B7C5", "#EBCC2A", "#E1AF00", "#F21A00"}},
	{Name: "Darjeeling1", Colors: []lipgloss.Color{"#FF0000", "#00A08A", "#F2AD00", "#F98400", "#5BBCD6"}},
	{Name: "Darjeeling2", Colors: []lipgloss.Color{"#ECCBAE", "#046C9A", "#D69C4E", "#ABDDDE", "#000000"}},
	{Name: "Chevalier1", Colors: []lipgloss.Color{"#446455", "#FDD262", "#D3DDDC", "#C7B19C"}},
	{Name: "FantasticFox1", Colors: []lipgloss.Color{"#DD8D29", "#E2D200", "#46ACC8", "#E58601", "#B40F20"}},
	{Name: "Moonrise1", Colors: []lipgloss.Color{"#F3DF6C", "#CEAB07", "#D5D5D3", "#24281A"}},
	{Name: "Moonrise2", Colors: []lipgloss.Color{"#798E87", "#C27D38", "#CCC591", "#29211F"}},
	{Name: "Moonrise3", Colors: []lipgloss.Color{"#85D4E3", "#F4B5BD", "#9C964A", "#CDC08C", "#FAD77B"}},
	{Name: "Cavalcanti1", Colors: []lipgloss.Color{"#D8B70A", "#02401B", "#A2A475", "#81A88D", "#972D15"}},
	{Name: "GrandBudapest1", Colors: []lipgloss.Color{"#F1BB7B", "#FD6467", "#5B1A18", "#D67236"}},
	{Name: "GrandBudapest2", Colors: []lipgloss.Color{"#E6A0C4", "#C6CDF7", "#D8A499", "#7294D4"}},
	{Name: "IsleofDogs1", Colors: []lipgloss.Color{"#9986A5", "#79402E", "#CCBA72", "#0F0D0E", "#D9D0D3", "#8D8680"}},
	{Name: "IsleofDogs2", Colors: []lipgloss.Color{"#EAD3BF", "#AA9486", "#B6854D", "#39312F", "#1C1718"}},
	{Name: "FrenchDispatch", Colors: []lipgloss.Color{"#90D4CC", "#BD3027", "#B0AFA2", "#7FC0C6", "#9D9C85"}},
	{Name: "AsteroidCity1", Colors: []lipgloss.Color{"#0A9F9D", "#CEB175", "#E54E21", "#6C8645", "#C18748"}},
	{Name: "AsteroidCity2", Colors: []lipgloss.Color{"#C52E19", "#AC9765", "#54D8B1", "#b67c3b", "#175149", "#AF4E24"}},
	{Name: "AsteroidCity3", Colors: []lipgloss.Color{"#FBA72A", "#D3D4D8", "#CB7A5C", "#5785C1"}},
}

// AllUserColors returns a flat list of all unique hex colors across all palettes,
// suitable for hashing user identities into distinct colors.
func AllUserColors() []lipgloss.Color {
	seen := make(map[lipgloss.Color]bool)
	var colors []lipgloss.Color
	for _, p := range WesAndersonPalettes {
		for _, c := range p.Colors {
			if !seen[c] {
				seen[c] = true
				colors = append(colors, c)
			}
		}
	}
	return colors
}

// SystemColor is the reserved color for system/AI messages.
// Grand Budapest Hotel muted rose — distinct from all user palette colors.
var SystemColor = lipgloss.Color("#7294D4")
