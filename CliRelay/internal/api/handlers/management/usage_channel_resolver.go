package management

import (
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageChannelResolver struct {
	keyNames            map[string]string
	latestByAuthIndex   map[string]string
	latestBySource      map[string]string
	rawNamesByDisplay   map[string][]string
	displayChannelNames []string
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
		keyNames:            keyNameMap,
		latestByAuthIndex:   make(map[string]string),
		latestBySource:      make(map[string]string, len(sourceNameMap)),
		rawNamesByDisplay:   make(map[string][]string),
		displayChannelNames: []string{},
	}

	for source, name := range sourceNameMap {
		source = strings.TrimSpace(source)
		name = strings.TrimSpace(name)
		if source == "" || name == "" {
			continue
		}
		resolver.latestBySource[source] = name
	}

	if h != nil && h.authManager != nil {
		for _, auth := range h.authManager.List() {
			if auth == nil {
				continue
			}

			idx := strings.TrimSpace(auth.EnsureIndex())
			name := strings.TrimSpace(auth.Label)
			if name == "" && auth.Attributes != nil {
				if apiKey := strings.TrimSpace(auth.Attributes["api_key"]); apiKey != "" {
					name = strings.TrimSpace(resolver.latestBySource[apiKey])
				}
			}
			if name == "" {
				continue
			}

			if idx != "" {
				resolver.latestByAuthIndex[idx] = name
			}
			if auth.Attributes != nil {
				if apiKey := strings.TrimSpace(auth.Attributes["api_key"]); apiKey != "" {
					resolver.latestBySource[apiKey] = name
				}
			}
		}
	}

	displaySet := make(map[string]struct{})
	for _, ref := range refs {
		displayName := resolver.ResolveDisplayName(ref.AuthIndex, ref.ChannelName, ref.Source)
		if displayName == "" {
			continue
		}
		displaySet[displayName] = struct{}{}
		rawName := strings.TrimSpace(ref.ChannelName)
		if rawName != "" {
			resolver.rawNamesByDisplay[displayName] = appendUniqueString(
				resolver.rawNamesByDisplay[displayName],
				rawName,
			)
		}
	}

	for displayName := range displaySet {
		resolver.displayChannelNames = append(resolver.displayChannelNames, displayName)
	}
	sort.Strings(resolver.displayChannelNames)
	for key := range resolver.rawNamesByDisplay {
		sort.Strings(resolver.rawNamesByDisplay[key])
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
		if name := strings.TrimSpace(r.latestByAuthIndex[authIndex]); name != "" {
			return name
		}
	}
	if source != "" {
		if name := strings.TrimSpace(r.latestBySource[source]); name != "" {
			return name
		}
	}
	if channelName != "" {
		return channelName
	}
	return source
}

func (r usageChannelResolver) ResolveRawChannelNames(selected []string) []string {
	resolved := make([]string, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		displayName := strings.TrimSpace(item)
		if displayName == "" {
			continue
		}

		rawNames := r.rawNamesByDisplay[displayName]
		if len(rawNames) == 0 {
			rawNames = []string{displayName}
		}
		for _, rawName := range rawNames {
			rawName = strings.TrimSpace(rawName)
			if rawName == "" {
				continue
			}
			if _, ok := seen[rawName]; ok {
				continue
			}
			seen[rawName] = struct{}{}
			resolved = append(resolved, rawName)
		}
	}
	return resolved
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
