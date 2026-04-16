package i18n

// PluralRu returns the correct Russian plural form for integer n.
//
//	n=1  → one  ("1 окно")
//	n=2  → few  ("2 окна")
//	n=5  → many ("5 окон")
//	n=11 → many ("11 окон")  — teens always take the many form
func PluralRu(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	mod100, mod10 := n%100, n%10
	if mod100 >= 11 && mod100 <= 19 {
		return many
	}
	switch mod10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
