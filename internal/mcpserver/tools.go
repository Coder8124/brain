package mcpserver

// The tools this server exposes to an MCP host. Small and sharp: an external
// agent should be able to remember something, recall what is relevant, see
// everything, and forget — the whole surface of a personal memory, nothing
// more. Descriptions are written for the host's model, telling it *when* to
// reach for each.

var toolDefs = []map[string]any{
	{
		"name":        "remember",
		"description": "Save something durable about the user to their private local memory — a preference, a fact about a person, standing context, or a decision. Use this whenever the user states something worth remembering across future conversations (e.g. 'I prefer short replies', 'my CFO is Sarah', 'we launch in Q4'). Stored on the user's machine, never uploaded.",
		"inputSchema": obj(map[string]any{
			"text": str("the thing to remember, as a clear standalone statement"),
			"kind": enumStr("what kind of memory it is", "preference", "person", "context", "fact"),
		}, "text"),
	},
	{
		"name":        "recall",
		"description": "Retrieve what is known about the user relevant to a query, from their private local memory. Use this at the start of a task or whenever the user's preferences, people, or prior context would help — so you act on what they've told you before instead of asking again.",
		"inputSchema": obj(map[string]any{
			"query": str("what you want to recall about the user"),
			"limit": intSchema("how many memories to return (default 5)"),
		}, "query"),
	},
	{
		"name":        "list_memories",
		"description": "List everything currently in the user's memory, with ids. Use to review or before forgetting something.",
		"inputSchema": obj(map[string]any{}),
	},
	{
		"name":        "forget",
		"description": "Delete a memory by its id (from list_memories). Use when the user asks to forget something or a memory is wrong.",
		"inputSchema": obj(map[string]any{"id": str("the memory id to forget")}, "id"),
	},
}

func obj(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func intSchema(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func enumStr(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}
