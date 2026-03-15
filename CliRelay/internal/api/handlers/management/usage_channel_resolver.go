package management

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

const (
	usageChannelAuthTokenPrefix       = "auth:"
	usageChannelLegacyNameTokenPrefix = "legacy-name:"
	usageChannelSourceTokenPrefix     = "source:"
)

type usageChannelResolver struct {
	keyNames                map[string]string
	displayByAuthIndex      map[string]string
	displayBySource         map[string]string
	ambiguousAuthIndex      map[string]struct{}
	ambiguousSource         map[string]struct{}
	sourcesByAuthIndex      map[string][]string
	channelNamesByAuthIndex map[string][]string
	filterByLabel           map[string]usage.ChannelFilter
	filterByValue           map[string]usage.ChannelFilter
	channelOptions          []usage.ChannelOption
	displayChannelNames     []string
}

func (h *Handler) buildUsageChannelResolver(params usage.LogQueryParams) (usageChannelResolver, error) {
	refs, err := usage.QueryChannelRefs(params)
	if err != nil {
		return usageChannelResolver{}, err
	}
	return h.newUsageChannelResolver(refs), nil
}

func (h *Handler) newUsageChannelResolver(refs []usage.ChannelRef) usageChannelResolver {
	keyNameMap, sourceNameMap := h.buildNameMaps()
	resolver := usageChannelResolver{
		keyNames:                keyNameMap,
		displayByAuthIndex:      make(map[string]string),
		displayBySource:         make(map[string]string, len(sourceNameMap)),
		ambiguousAuthIndex:      make(map[string]struct{}),
		ambiguousSource:         make(map[string]struct{}),
		sourcesByAuthIndex:      make(map[string][]string),
		channelNamesByAuthIndex: make(map[string][]string),
		filterByLabel:           make(map[string]usage.ChannelFilter),
		filterByValue:           make(map[string]usage.ChannelFilter),
		channelOptions:          []usage.ChannelOption{},
		displayChannelNames:     []string{},
	}

	for source, name := range sourceNameMap {
		source = strings.TrimSpace(source)
		name = strings.TrimSpace(name)
		if source == "" || name == "" {
			continue
		}
		resolver.assignSourceDisplay(source, name)
	}

	if h != nil && h.authManager != nil {
		auths := h.authManager.List()
		sort.SliceStable(auths, func(i, j int) bool {
			left := auths[i]
			right := auths[j]
			leftIndex, rightIndex := "", ""
			leftSource, rightSource := "", ""
			leftLabel, rightLabel := "", ""
			leftProvider, rightProvider := "", ""
			leftID, rightID := "", ""
			if left != nil {
				leftIndex = strings.TrimSpace(left.EnsureIndex())
				leftSource = resolveUsageChannelStableSource(left)
				leftLabel = strings.TrimSpace(left.Label)
				leftProvider = strings.TrimSpace(left.Provider)
				leftID = strings.TrimSpace(left.ID)
			}
			if right != nil {
				rightIndex = strings.TrimSpace(right.EnsureIndex())
				rightSource = resolveUsageChannelStableSource(right)
				rightLabel = strings.TrimSpace(right.Label)
				rightProvider = strings.TrimSpace(right.Provider)
				rightID = strings.TrimSpace(right.ID)
			}
			if leftIndex != rightIndex {
				return leftIndex < rightIndex
			}
			if leftSource != rightSource {
				return leftSource < rightSource
			}
			if leftLabel != rightLabel {
				return leftLabel < rightLabel
			}
			if leftProvider != rightProvider {
				return leftProvider < rightProvider
			}
			return leftID < rightID
		})

		for _, auth := range auths {
			if auth == nil {
				continue
			}

			authIndex := strings.TrimSpace(auth.EnsureIndex())
			source := resolveUsageChannelStableSource(auth)
			label := strings.TrimSpace(auth.Label)
			if label == "" && source != "" {
				label = strings.TrimSpace(sourceNameMap[source])
			}
			if label == "" {
				continue
			}

			if authIndex != "" {
				resolver.assignAuthDisplay(authIndex, label)
			}
			if source != "" {
				resolver.assignSourceDisplay(source, label)
				if authIndex != "" {
					resolver.sourcesByAuthIndex[authIndex] = appendUniqueString(
						resolver.sourcesByAuthIndex[authIndex],
						source,
					)
				}
			}
		}
	}

	authLabelByIndex := make(map[string]string)
	for _, ref := range refs {
		authIndex := strings.TrimSpace(ref.AuthIndex)
		channelName := strings.TrimSpace(ref.ChannelName)
		source := strings.TrimSpace(ref.Source)
		if authIndex == "" {
			continue
		}
		if channelName != "" {
			resolver.channelNamesByAuthIndex[authIndex] = appendUniqueString(
				resolver.channelNamesByAuthIndex[authIndex],
				channelName,
			)
		}
		if source != "" {
			resolver.sourcesByAuthIndex[authIndex] = appendUniqueString(
				resolver.sourcesByAuthIndex[authIndex],
				source,
			)
		}
		if _, exists := authLabelByIndex[authIndex]; exists {
			continue
		}
		label := resolver.resolveOptionLabel(ref)
		if label == "" {
			continue
		}
		authLabelByIndex[authIndex] = label
	}

	usedLabels := make(map[string]struct{})
	authOptionValues := make(map[string]struct{})
	sourceOptionValues := make(map[string]struct{})
	addAuthOption := func(authIndex, label string) {
		authIndex = strings.TrimSpace(authIndex)
		label = strings.TrimSpace(label)
		if authIndex == "" || label == "" {
			return
		}

		value := makeUsageAuthChannelToken(authIndex)
		if value == "" {
			return
		}
		if _, exists := authOptionValues[value]; exists {
			return
		}
		authOptionValues[value] = struct{}{}

		filter := usage.ChannelFilter{
			AuthIndexes: []string{authIndex},
		}
		for _, channelName := range resolver.channelNamesByAuthIndex[authIndex] {
			filter.ChannelNames = appendUniqueString(filter.ChannelNames, channelName)
		}
		for _, source := range resolver.sourcesByAuthIndex[authIndex] {
			filter.Sources = appendUniqueString(filter.Sources, source)
			if sourceValue := makeUsageSourceChannelToken(source); sourceValue != "" {
				sourceOptionValues[sourceValue] = struct{}{}
			}
		}
		if len(filter.ChannelNames) == 0 {
			filter.ChannelNames = appendUniqueString(filter.ChannelNames, label)
		}

		optionLabel := uniqueUsageChannelOptionLabel(label, "auth", usedLabels)
		resolver.addChannelOption(value, optionLabel, filter)
	}
	addSourceOption := func(source, label string, channelNames ...string) {
		source = strings.TrimSpace(source)
		label = strings.TrimSpace(label)
		if source == "" || label == "" {
			return
		}

		value := makeUsageSourceChannelToken(source)
		if value == "" {
			return
		}
		if _, exists := sourceOptionValues[value]; exists {
			return
		}
		sourceOptionValues[value] = struct{}{}

		filter := usage.ChannelFilter{
			Sources: []string{source},
		}
		for _, channelName := range channelNames {
			filter.ChannelNames = appendUniqueString(filter.ChannelNames, channelName)
		}
		if len(filter.ChannelNames) == 0 {
			filter.ChannelNames = appendUniqueString(filter.ChannelNames, label)
		}

		optionLabel := uniqueUsageChannelOptionLabel(label, "source", usedLabels)
		resolver.addChannelOption(value, optionLabel, filter)
	}
	for _, ref := range refs {
		authIndex := strings.TrimSpace(ref.AuthIndex)
		if authIndex == "" {
			continue
		}
		label := strings.TrimSpace(authLabelByIndex[authIndex])
		addAuthOption(authIndex, label)
	}

	legacyOptionValues := make(map[string]struct{})
	for _, ref := range refs {
		if strings.TrimSpace(ref.AuthIndex) != "" {
			continue
		}
		channelName := strings.TrimSpace(ref.ChannelName)
		value := makeUsageLegacyNameToken(channelName)
		if value == "" {
			continue
		}
		if _, exists := legacyOptionValues[value]; exists {
			continue
		}
		legacyOptionValues[value] = struct{}{}
		optionLabel := uniqueUsageChannelOptionLabel(channelName, "legacy", usedLabels)
		resolver.addChannelOption(value, optionLabel, usage.ChannelFilter{
			ChannelNames: []string{channelName},
		})
	}
	for _, ref := range refs {
		if strings.TrimSpace(ref.AuthIndex) != "" || strings.TrimSpace(ref.ChannelName) != "" {
			continue
		}
		source := strings.TrimSpace(ref.Source)
		label := strings.TrimSpace(resolver.displayBySource[source])
		if label == "" {
			continue
		}
		addSourceOption(source, label)
	}
	for authIndex, label := range resolver.displayByAuthIndex {
		addAuthOption(authIndex, label)
	}
	for source, label := range resolver.displayBySource {
		addSourceOption(source, label)
	}

	sort.SliceStable(resolver.channelOptions, func(i, j int) bool {
		left := resolver.channelOptions[i]
		right := resolver.channelOptions[j]
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.Value < right.Value
	})

	resolver.displayChannelNames = make([]string, 0, len(resolver.channelOptions))
	for _, option := range resolver.channelOptions {
		resolver.displayChannelNames = append(resolver.displayChannelNames, option.Label)
	}

	return resolver
}

func (r usageChannelResolver) ResolveAPIKeyName(apiKey string) string {
	return strings.TrimSpace(r.keyNames[strings.TrimSpace(apiKey)])
}

func (r usageChannelResolver) ResolveDisplayName(authIndex, channelName, source string) string {
	authIndex = strings.TrimSpace(authIndex)
	channelName = strings.TrimSpace(channelName)
	source = strings.TrimSpace(source)

	if authIndex != "" {
		if _, ambiguous := r.ambiguousAuthIndex[authIndex]; !ambiguous {
			if label := strings.TrimSpace(r.displayByAuthIndex[authIndex]); label != "" {
				return label
			}
		}
	}
	if channelName != "" {
		return channelName
	}
	if source != "" {
		if _, ambiguous := r.ambiguousSource[source]; !ambiguous {
			if label := strings.TrimSpace(r.displayBySource[source]); label != "" {
				return label
			}
		}
	}
	return ""
}

func (r *usageChannelResolver) assignAuthDisplay(authIndex, label string) {
	authIndex = strings.TrimSpace(authIndex)
	label = strings.TrimSpace(label)
	if r == nil || authIndex == "" || label == "" {
		return
	}
	if _, ambiguous := r.ambiguousAuthIndex[authIndex]; ambiguous {
		return
	}
	if existing := strings.TrimSpace(r.displayByAuthIndex[authIndex]); existing != "" && existing != label {
		delete(r.displayByAuthIndex, authIndex)
		r.ambiguousAuthIndex[authIndex] = struct{}{}
		return
	}
	r.displayByAuthIndex[authIndex] = label
}

func (r *usageChannelResolver) assignSourceDisplay(source, label string) {
	source = strings.TrimSpace(source)
	label = strings.TrimSpace(label)
	if r == nil || source == "" || label == "" {
		return
	}
	if _, ambiguous := r.ambiguousSource[source]; ambiguous {
		return
	}
	if existing := strings.TrimSpace(r.displayBySource[source]); existing != "" && existing != label {
		delete(r.displayBySource, source)
		r.ambiguousSource[source] = struct{}{}
		return
	}
	r.displayBySource[source] = label
}

func (r usageChannelResolver) ResolveChannelFilter(selected []string) usage.ChannelFilter {
	filter := usage.ChannelFilter{}
	for _, item := range selected {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}

		if authIndex, ok := parseUsageAuthChannelToken(value); ok {
			r.appendAuthChannelSelection(&filter, authIndex)
			continue
		}
		if channelName, ok := parseUsageLegacyNameToken(value); ok {
			appendUsageChannelFilter(&filter, usage.ChannelFilter{
				ChannelNames: []string{channelName},
			})
			continue
		}

		if mapped, ok := r.filterByValue[value]; ok {
			appendUsageChannelFilter(&filter, mapped)
			continue
		}
		if source, ok := parseUsageSourceChannelToken(value); ok {
			appendUsageChannelFilter(&filter, usage.ChannelFilter{
				Sources: []string{source},
			})
			continue
		}

		if mapped, ok := r.filterByLabel[value]; ok {
			appendUsageChannelFilter(&filter, mapped)
			continue
		}

		filter.ChannelNames = appendUniqueString(filter.ChannelNames, value)
	}
	return filter
}

func (r usageChannelResolver) appendAuthChannelSelection(filter *usage.ChannelFilter, authIndex string) {
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" || filter == nil {
		return
	}
	filter.AuthIndexes = appendUniqueString(filter.AuthIndexes, authIndex)
	for _, channelName := range r.channelNamesByAuthIndex[authIndex] {
		filter.ChannelNames = appendUniqueString(filter.ChannelNames, channelName)
	}
	for _, source := range r.sourcesByAuthIndex[authIndex] {
		filter.Sources = appendUniqueString(filter.Sources, source)
	}
}

func (r *usageChannelResolver) addChannelOption(value, label string, filter usage.ChannelFilter) {
	value = strings.TrimSpace(value)
	label = strings.TrimSpace(label)
	if value == "" || label == "" || r == nil {
		return
	}
	r.channelOptions = append(r.channelOptions, usage.ChannelOption{
		Value: value,
		Label: label,
	})
	r.filterByLabel[label] = cloneUsageChannelFilter(filter)
	r.filterByValue[value] = cloneUsageChannelFilter(filter)
}

func (r usageChannelResolver) resolveOptionLabel(ref usage.ChannelRef) string {
	authIndex := strings.TrimSpace(ref.AuthIndex)
	if authIndex != "" {
		if label := strings.TrimSpace(r.displayByAuthIndex[authIndex]); label != "" {
			return label
		}
	}
	if channelName := strings.TrimSpace(ref.ChannelName); channelName != "" {
		return channelName
	}
	return ""
}

func resolveUsageChannelStableSource(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	provider := strings.TrimSpace(auth.Provider)
	if strings.EqualFold(provider, "gemini-cli") {
		if id := strings.TrimSpace(auth.ID); id != "" {
			return id
		}
	}
	if strings.EqualFold(provider, "vertex") {
		if auth.Metadata != nil {
			if projectID, ok := auth.Metadata["project_id"].(string); ok {
				if trimmed := strings.TrimSpace(projectID); trimmed != "" {
					return trimmed
				}
			}
			if project, ok := auth.Metadata["project"].(string); ok {
				if trimmed := strings.TrimSpace(project); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if kind, value := auth.AccountInfo(); strings.EqualFold(kind, "api_key") {
		return strings.TrimSpace(value)
	}
	if auth.Attributes != nil {
		if apiKey := strings.TrimSpace(auth.Attributes["api_key"]); apiKey != "" {
			return apiKey
		}
	}
	return ""
}

func makeUsageAuthChannelToken(authIndex string) string {
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return ""
	}
	return usageChannelAuthTokenPrefix + authIndex
}

func parseUsageAuthChannelToken(value string) (string, bool) {
	if !strings.HasPrefix(value, usageChannelAuthTokenPrefix) {
		return "", false
	}
	authIndex := strings.TrimSpace(strings.TrimPrefix(value, usageChannelAuthTokenPrefix))
	if authIndex == "" {
		return "", false
	}
	return authIndex, true
}

func makeUsageLegacyNameToken(channelName string) string {
	channelName = strings.TrimSpace(channelName)
	if channelName == "" {
		return ""
	}
	return usageChannelLegacyNameTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(channelName))
}

func parseUsageLegacyNameToken(value string) (string, bool) {
	if !strings.HasPrefix(value, usageChannelLegacyNameTokenPrefix) {
		return "", false
	}
	encoded := strings.TrimSpace(strings.TrimPrefix(value, usageChannelLegacyNameTokenPrefix))
	if encoded == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	channelName := strings.TrimSpace(string(decoded))
	if channelName == "" {
		return "", false
	}
	return channelName, true
}

func makeUsageSourceChannelToken(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	return usageChannelSourceTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(source))
}

func parseUsageSourceChannelToken(value string) (string, bool) {
	if !strings.HasPrefix(value, usageChannelSourceTokenPrefix) {
		return "", false
	}
	encoded := strings.TrimSpace(strings.TrimPrefix(value, usageChannelSourceTokenPrefix))
	if encoded == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	source := strings.TrimSpace(string(decoded))
	if source == "" {
		return "", false
	}
	return source, true
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUsageChannelFilter(target *usage.ChannelFilter, addition usage.ChannelFilter) {
	if target == nil {
		return
	}
	for _, authIndex := range addition.AuthIndexes {
		target.AuthIndexes = appendUniqueString(target.AuthIndexes, authIndex)
	}
	for _, channelName := range addition.ChannelNames {
		target.ChannelNames = appendUniqueString(target.ChannelNames, channelName)
	}
	for _, source := range addition.Sources {
		target.Sources = appendUniqueString(target.Sources, source)
	}
}

func cloneUsageChannelFilter(filter usage.ChannelFilter) usage.ChannelFilter {
	cloned := usage.ChannelFilter{}
	if len(filter.AuthIndexes) > 0 {
		cloned.AuthIndexes = append([]string{}, filter.AuthIndexes...)
	}
	if len(filter.ChannelNames) > 0 {
		cloned.ChannelNames = append([]string{}, filter.ChannelNames...)
	}
	if len(filter.Sources) > 0 {
		cloned.Sources = append([]string{}, filter.Sources...)
	}
	return cloned
}

func uniqueUsageChannelOptionLabel(baseLabel, kind string, used map[string]struct{}) string {
	baseLabel = strings.TrimSpace(baseLabel)
	if baseLabel == "" {
		return ""
	}
	if used == nil {
		return baseLabel
	}
	if _, exists := used[baseLabel]; !exists {
		used[baseLabel] = struct{}{}
		return baseLabel
	}

	label := baseLabel
	switch kind {
	case "legacy":
		label = baseLabel + " [legacy]"
	default:
		label = baseLabel
	}
	if _, exists := used[label]; !exists {
		used[label] = struct{}{}
		return label
	}

	for index := 2; ; index++ {
		var candidate string
		if kind == "legacy" {
			candidate = fmt.Sprintf("%s [legacy %d]", baseLabel, index)
		} else {
			candidate = fmt.Sprintf("%s [%d]", baseLabel, index)
		}
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}
