package nativetools

import (
	"regexp"
	"sync"
)

// regexpCache memoizes compiled patterns. Template engines re-evaluate the
// same matchers/extractors for every probe response; recompiling the same
// pattern per request is pure waste (the audit measured dozens of Compile
// calls per target scan). Patterns come from templates (bounded set);
// a failed compile is cached as nil so broken patterns fail fast too.
var regexpCache sync.Map // string -> *regexp.Regexp (nil on compile error)

// compileRegexp returns the cached compiled pattern (nil when invalid).
func compileRegexp(pattern string) *regexp.Regexp {
	if cached, ok := regexpCache.Load(pattern); ok {
		compiled, _ := cached.(*regexp.Regexp)
		return compiled
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		compiled = nil
	}
	regexpCache.Store(pattern, compiled)
	return compiled
}

// regexpMustMatches reports whether pattern matches subject; invalid or
// non-matching patterns both return false (mirrors the previous inline
// `err != nil || !re.MatchString(...)` contract).
func regexpMustMatches(pattern, subject string) bool {
	re := compileRegexp(pattern)
	return re != nil && re.MatchString(subject)
}
