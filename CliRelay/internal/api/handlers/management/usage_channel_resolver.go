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
	usageChannelAuthTokenPrefix       = "auth-id:"
	usageChannelLegacyAuthTokenPrefix = "auth:"
	usageChannelLegacyNameTokenPrefix = "legacy-name:"
	usageChannelSourceTokenPrefix     = "source:"
)

type usageChannelResolver struct {
	keyNames                map[string]string
	displayByAuthID         map[string]string
	displayByProviderSource map[string]string
	displayByAuthIndex      map[string]string
	displayBySource         map[string]string
	ambiguousProviderSource map[string]struct{}
	ambiguousAuthIndex      map[string]struct{}
	ambiguousSource         map[string]struct{}
	authIndexesByAuthID     map[string][]string
	sourcesByAuthID         map[string][]string
	channelNamesByAuthID    map[string][]string
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
		displayByAuthID:         make(map[string]string),
		displayByProviderSource: make(map[string]string),
		displayByAuthIndex:      make(map[string]string),
		displayBySource:         make(map[string]string, len(sourceNameMap)),
		ambiguousProviderSource: make(map[string]struct{}),
		ambiguousAuthIndex:      make(map[string]struct{}),
		ambiguousSource:         make(map[string]struct{}),
		authIndexesByAuthID:     make(map[string][]string),
		sourcesByAuthID:         make(map[string][]string),
		channelNamesByAuthID:    make(map[string][]string),
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
			leftAuthID, rightAuthID := "", ""
			leftIndex, rightIndex := "", ""
			leftSource, rightSource := "", ""
			leftLabel, rightLabel := "", ""
			leftProvider, rightProvider := "", ""
			if left != nil {
				leftAuthID = strings.TrimSpace(left.ID)
				leftIndex = strings.TrimSpace(left.EnsureIndex())
				leftSource = resolveUsageChannelStableSource(left)
				leftLabel = strings.TrimSpace(left.Label)
				leftProvider = strings.TrimSpace(left.Provider)
			}
			if right != nil {
				rightAuthID = strings.TrimSpace(right.ID)
				rightIndex = strings.TrimSpace(right.EnsureIndex())
				rightSource = resolveUsageChannelStableSource(right)
				rightLabel = strings.TrimSpace(right.Label)
				rightProvider = strings.TrimSpace(right.Provider)
			}
			if leftAuthID != rightAuthID {
				return leftAuthID < rightAuthID
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
			return false
		})

		for _, auth := range auths {
			if auth == nil {
				continue
			}

			authID := strings.TrimSpace(auth.ID)
			authIndex := strings.TrimSpace(auth.EnsureIndex())
			source := resolveUsageChannelStableSource(auth)
			provider := strings.TrimSpace(auth.Provider)
			label := strings.TrimSpace(auth.Label)
			if label == "" && source != "" {
				label = strings.TrimSpace(sourceNameMap[source])
			}
			if label == "" {
				continue
			}

			if authID != "" {
				resolver.assignAuthIDDisplay(authID, label)
				resolver.channelNamesByAuthID[authID] = appendUniqueString(
					resolver.channelNamesByAuthID[authID],
					label,
				)
				if authIndex != "" {
					resolver.authIndexesByAuthID[authID] = appendUniqueString(
						resolver.authIndexesByAuthID[authID],
						authIndex,
					)
				}
				if source != "" {
					resolver.sourcesByAuthID[authID] = appendUniqueString(
						resolver.sourcesByAuthID[authID],
						source,
					)
				}
			}
			if authIndex != "" {
				resolver.assignAuthDisplay(authIndex, label)
			}
			if provider != "" && source != "" {
				resolver.assignProviderSourceDisplay(provider, source, label)
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

	authLabelByID := make(map[string]string)
	authLabelByIndex := make(map[string]string)
	for _, ref := range refs {
		authID := strings.TrimSpace(ref.AuthID)
		authIndex := strings.TrimSpace(ref.AuthIndex)
		channelName := strings.TrimSpace(ref.ChannelName)
		source := strings.TrimSpace(ref.Source)
		if authID != "" {
			if authIndex != "" {
				resolver.authIndexesByAuthID[authID] = appendUniqueString(
					resolver.authIndexesByAuthID[authID],
					authIndex,
				)
			}
			if channelName != "" {
				resolver.channelNamesByAuthID[authID] = appendUniqueString(
					resolver.channelNamesByAuthID[authID],
					channelName,
				)
			}
			if source != "" {
				resolver.sourcesByAuthID[authID] = appendUniqueString(
					resolver.sourcesByAuthID[authID],
					source,
				)
			}
			if _, exists := authLabelByID[authID]; !exists {
				if label := resolver.resolveOptionLabel(ref); label != "" {
					authLabelByID[authID] = label
				}
			}
		}
		if authIndex != "" && channelName != "" {
			resolver.channelNamesByAuthIndex[authIndex] = appendUniqueString(
				resolver.channelNamesByAuthIndex[authIndex],
				channelName,
			)
		}
		if authIndex != "" && source != "" {
			resolver.sourcesByAuthIndex[authIndex] = appendUniqueString(
				resolver.sourcesByAuthIndex[authIndex],
				source,
			)
		}
		if authIndex == "" {
			continue
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
	addAuthOption := func(authID, label string) {
		authID = strings.TrimSpace(authID)
		label = strings.TrimSpace(label)
		if authID == "" || label == "" {
			return
		}

		value := makeUsageAuthChannelToken(authID)
		if value == "" {
			return
		}
		if _, exists := authOptionValues[value]; exists {
			return
		}
		authOptionValues[value] = struct{}{}

		filter := usage.ChannelFilter{
			AuthIDs: []string{authID},
		}
		for _, channelName := range resolver.channelNamesByAuthID[authID] {
			filter.ChannelNames = appendUniqueString(filter.ChannelNames, channelName)
		}
		for _, authIndex := range resolver.authIndexesByAuthID[authID] {
			filter.AuthIndexes = appendUniqueString(filter.AuthIndexes, authIndex)
		}
		for _, source := range resolver.sourcesByAuthID[authID] {
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
		authID := strings.TrimSpace(ref.AuthID)
		if authID == "" {
			continue
		}
		label := strings.TrimSpace(authLabelByID[authID])
		addAuthOption(authID, label)
	}

	legacyOptionValues := make(map[string]struct{})
	for _, ref := range refs {
		if strings.TrimSpace(ref.AuthID) != "" {
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
		if strings.TrimSpace(ref.AuthID) != "" || strings.TrimSpace(ref.ChannelName) != "" {
			continue
		}
		source := strings.TrimSpace(ref.Source)
		label := strings.TrimSpace(resolver.displayBySource[source])
		if label == "" {
			continue
		}
		addSourceOption(source, label)
	}
	for authID, label := range resolver.displayByAuthID {
		addAuthOption(authID, label)
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

func (r usageChannelResolver) ResolveDisplayName(authID, authIndex, channelName, source, provider string) string {
	authID = strings.TrimSpace(authID)
	authIndex = strings.TrimSpace(authIndex)
	channelName = strings.TrimSpace(channelName)
	source = strings.TrimSpace(source)
	provider = strings.TrimSpace(provider)

	if authID != "" {
		if label := strings.TrimSpace(r.displayByAuthID[authID]); label != "" {
			return label
		}
	}
	if provider != "" && source != "" {
		if label := r.resolveProviderSourceDisplay(provider, source); label != "" {
			return label
		}
	}

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

func (r *usageChannelResolver) assignAuthIDDisplay(authID, label string) {
	authID = strings.TrimSpace(authID)
	label = strings.TrimSpace(label)
	if r == nil || authID == "" || label == "" {
		return
	}
	if existing := strings.TrimSpace(r.displayByAuthID[authID]); existing != "" {
		return
	}
	r.displayByAuthID[authID] = label
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

func usageProviderSourceKey(provider, source string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	source = strings.TrimSpace(source)
	if provider == "" || source == "" {
		return ""
	}
	return provider + "\x00" + source
}

func (r *usageChannelResolver) assignProviderSourceDisplay(provider, source, label string) {
	key := usageProviderSourceKey(provider, source)
	label = strings.TrimSpace(label)
	if r == nil || key == "" || label == "" {
		return
	}
	if _, ambiguous := r.ambiguousProviderSource[key]; ambiguous {
		return
	}
	if existing := strings.TrimSpace(r.displayByProviderSource[key]); existing != "" && existing != label {
		delete(r.displayByProviderSource, key)
		r.ambiguousProviderSource[key] = struct{}{}
		return
	}
	r.displayByProviderSource[key] = label
}

func (r usageChannelResolver) resolveProviderSourceDisplay(provider, source string) string {
	key := usageProviderSourceKey(provider, source)
	if key == "" {
		return ""
	}
	if _, ambiguous := r.ambiguousProviderSource[key]; ambiguous {
		return ""
	}
	return strings.TrimSpace(r.displayByProviderSource[key])
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

		if authID, ok := parseUsageAuthChannelToken(value); ok {
			r.appendAuthIDChannelSelection(&filter, authID)
			continue
		}
		if authIndex, ok := parseUsageLegacyAuthChannelToken(value); ok {
			r.appendAuthIndexChannelSelection(&filter, authIndex)
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

func (r usageChannelResolver) appendAuthIDChannelSelection(filter *usage.ChannelFilter, authID string) {
	authID = strings.TrimSpace(authID)
	if authID == "" || filter == nil {
		return
	}
	filter.AuthIDs = appendUniqueString(filter.AuthIDs, authID)
	for _, authIndex := range r.authIndexesByAuthID[authID] {
		filter.AuthIndexes = appendUniqueString(filter.AuthIndexes, authIndex)
	}
	for _, channelName := range r.channelNamesByAuthID[authID] {
		filter.ChannelNames = appendUniqueString(filter.ChannelNames, channelName)
	}
	for _, source := range r.sourcesByAuthID[authID] {
		filter.Sources = appendUniqueString(filter.Sources, source)
	}
}

func (r usageChannelResolver) appendAuthIndexChannelSelection(filter *usage.ChannelFilter, authIndex string) {
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
	authID := strings.TrimSpace(ref.AuthID)
	if authID != "" {
		if label := strings.TrimSpace(r.displayByAuthID[authID]); label != "" {
			return label
		}
	}
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
	authID := strings.TrimSpace(authIndex)
	if authID == "" {
		return ""
	}
	return usageChannelAuthTokenPrefix + authID
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

func parseUsageLegacyAuthChannelToken(value string) (string, bool) {
	if !strings.HasPrefix(value, usageChannelLegacyAuthTokenPrefix) {
		return "", false
	}
	authIndex := strings.TrimSpace(strings.TrimPrefix(value, usageChannelLegacyAuthTokenPrefix))
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
	for _, authID := range addition.AuthIDs {
		target.AuthIDs = appendUniqueString(target.AuthIDs, authID)
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
	if len(filter.AuthIDs) > 0 {
		cloned.AuthIDs = append([]string{}, filter.AuthIDs...)
	}
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
