# CBA Just-In-Time SKILLS Pattern

## Overview

The SKILLS pattern is a design approach where AI agents query context on demand rather than loading everything upfront. This reduces token usage by 99.2%.

## Anti-Pattern: Context Overload

```
Agent connects to all MCP servers upfront
→ Receives 200,000 tokens of documentation
→ Can't focus, irrelevant data, slow inference
→ 0.8% of context is actually relevant
```

## Correct Pattern: Just-In-Time

```
Agent identifies need: "authorization check patterns"
→ Invokes skill: QueryCBAGuidelines("authorization")
→ Receives 500 tokens of targeted content
→ 100% relevant, fast inference
→ Context discarded after use
```

## The 5 CBA SKILLS

### 1. QueryCBAGuidelines
- **When**: Before generating code, validating code, uncertain about CBA conventions
- **Input**: topic (e.g., "authorization"), context (e.g., "writing new class")
- **Output**: Guideline text + code examples + anti-patterns (~500 tokens)

### 2. QueryCBAExamples
- **When**: Learning CBA patterns, implementing similar functionality
- **Input**: Pattern description, object type
- **Output**: Top 3 real code examples with quality ratings (~1,000 tokens)

### 3. ValidateAgainstCBAStandards
- **When**: After code generation, before committing, during review
- **Input**: Source code to validate
- **Output**: Compliance score + violations with severity + fix suggestions

### 4. QueryDB3Object
- **When**: Analyzing dependencies, studying existing implementations
- **Input**: Object name, type
- **Output**: Object structure, methods, dependencies (~2,000 tokens)

### 5. LearnFromCBAIncidents
- **When**: Before generating code, investigating similar issues
- **Input**: Error pattern or code area
- **Output**: Related incidents + root causes + prevention patterns (~500 tokens)

## Token Efficiency

| Approach | Tokens | Cost | Relevance |
|----------|--------|------|-----------|
| Direct MCP (all context) | 200,000 | $0.60/request | 0.8% relevant |
| SKILLS (on-demand) | 1,600 | $0.005/request | 100% relevant |
| **Savings** | **99.2%** | **99.2%** | **Signal, not noise** |

## Implementation Notes

These SKILLS are a design pattern — they describe how VSP tools should be queried in CBA environments. The actual MCP tools (`GetSource`, `SearchObject`, `GrepObjects`, etc.) serve as the execution layer. The SKILLS pattern guides WHEN and HOW to query, not what tools exist.

In practice:
- "QueryCBAGuidelines" = targeted `GrepObjects` for coding patterns + `GetSource` for examples
- "ValidateAgainstCBAStandards" = `SyntaxCheck` + `RunATCCheck` + pattern matching
- "QueryDB3Object" = `SearchObject` + `GetSource` + `GetObjectStructure`
