package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Ready-made requests that chain the tools; clients like Claude Code list them as slash commands.
// In the texts, {name} is replaced by the argument of that name.
var prompts = []struct {
	name, title, description string
	args                     []string
	text                     string
}{
	{"cluster_health", "Cluster health check", "Look for problems in a Kafka cluster: brokers, replication, topics and consumer lag.", []string{"cluster"}, `
Check the health of the Kafka cluster {cluster} with the KafkaLens tools.
1. cluster_overview: brokers, controller, under-replicated and offline partitions.
2. list_topics: topics with errors or under-replicated partitions.
3. list_consumer_groups: groups with high lag, or not in the Stable or Empty state.
End with the problems found, the worst first, and what to look at next for each.`},

	{"inspect_topic", "Inspect a topic", "Explain how a topic is set up and what its messages look like.", []string{"cluster", "topic"}, `
Describe the topic {topic} of the Kafka cluster {cluster} with the KafkaLens tools.
1. describe_topic: partitions, replication and in-sync replicas, the configs that differ from the default (retention, cleanup.policy...), the consumer groups reading it and their lag.
2. browse_messages with from=latest: the format of keys and values (JSON and its fields, Avro or Protobuf when binary, plain text), how keys spread over partitions, the headers used.
Point out anything unusual: skewed or empty partitions, very large messages, groups far behind.`},

	{"diagnose_lag", "Diagnose consumer lag", "Find out why a consumer group is behind.", []string{"cluster", "group"}, `
Find out why the consumer group {group} of the Kafka cluster {cluster} is lagging, with the KafkaLens tools.
1. describe_consumer_group: state, members and their partitions, lag per partition. Look for partitions with lag and no member, lag piled on a few partitions (hot keys, one slow consumer), committed offsets that don't move between two calls a few seconds apart.
2. describe_topic for the topics involved: partitions, configs, other groups reading them.
3. For the worst partition, browse_messages with from=offset at its committed offset, to see the messages the group is stuck on.
End with the most likely causes and what to do. The KafkaLens MCP tools only read: offset resets are done from the KafkaLens UI.`},
}

func addPrompts(s *mcp.Server) {
	for _, p := range prompts {
		var args []*mcp.PromptArgument
		for _, a := range p.args {
			args = append(args, &mcp.PromptArgument{Name: a, Required: true, Description: map[string]string{
				"cluster": "cluster id, or its name", "topic": "topic name", "group": "consumer group id"}[a]})
		}
		s.AddPrompt(&mcp.Prompt{Name: p.name, Title: p.title, Description: p.description, Arguments: args},
			func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				text := strings.TrimSpace(p.text)
				for _, a := range p.args {
					v := req.Params.Arguments[a]
					if v == "" {
						return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf("argument %s is required", a)}
					}
					text = strings.ReplaceAll(text, "{"+a+"}", v)
				}
				return &mcp.GetPromptResult{Description: p.description,
					Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}}, nil
			})
	}
}
