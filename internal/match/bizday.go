package match

import "github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"

// bizIndex numbers business days consecutively, so the business-day gap between two dates is
// a subtraction. Weekend days share the index of the following Monday: Friday to Monday is one
// business day, as is Friday to Saturday. Holiday calendars are not modelled yet.
func bizIndex(d domain.Date) int32 {
	m := int32(d) + 3 // 1970-01-01 was a Thursday, so day 0 of this count is a Monday
	week, dow := m/7, m%7
	if dow < 0 {
		week, dow = week-1, dow+7
	}
	return week*5 + min(dow, 5)
}

// BizGap is the absolute number of business days between two dates.
func BizGap(a, b domain.Date) int32 {
	g := bizIndex(a) - bizIndex(b)
	if g < 0 {
		return -g
	}
	return g
}
