# Adaptive Software Design Workspace

> Status: implementation-ready design specification  
> Version: 0.1  
> Intended audience: developers and coding agents implementing this project  
> Normative terms: **MUST**, **SHOULD**, and **MAY** indicate requirement strength.

## 1. Executive Summary

Adaptive Software Design Workspace is a local-first web application that turns a vague software idea or change request into a reviewed, traceable, implementation-ready engineering model.

The product is not a general-purpose assistant, a coding agent, a project tracker, or a document generator. Its purpose is to support the reasoning that happens before and around implementation:

- understand the problem and desired outcome;
- identify stakeholders, boundaries, constraints, assumptions, and unknowns;
- derive scenarios, requirements, and measurable quality expectations;
- investigate high-risk unknowns;
- compare meaningful solution options and record decisions;
- produce an implementation and verification strategy;
- expose conflicts and unresolved risks for human approval;
- render only the documents and diagrams justified by the project.

The canonical source of truth is a structured **Project Model**, not a fixed collection of Markdown files. Documents are generated views over that model. This distinction is the main mechanism for supporting different project types without forcing irrelevant artifacts on them.

The default implementation is:

- Go modular monolith backend;
- SQLite persistence;
- REST commands and queries;
- Server-Sent Events (SSE) for live run updates;
- React and TypeScript web frontend;
- provider-neutral LLM adapter;
- deterministic workflow controller coordinating specialized agents;
- local single-user deployment for the first release.

## 2. Problem Statement

Coding agents can implement concrete instructions, but individual developers often begin with requests such as:

> Build a small service that helps students organize assignments.

This is insufficient for responsible implementation. It leaves unanswered questions about users, outcomes, scope, workflows, constraints, quality, security, data, failure behavior, and verification. A developer can either make untracked assumptions or spend substantial time reconstructing product management, requirements engineering, architecture, and delivery planning practices.

Existing approaches tend to fail in one of two ways:

1. They produce a fixed template containing irrelevant sections and false precision.
2. They allow multiple agents to discuss freely, generating long transcripts without accountable decisions or traceability.

This product addresses both failures with a typed knowledge model, risk-driven workflow, explicit human approvals, and dynamically selected engineering views.

## 3. Product Goals

### 3.1 Primary goal

Given a vague software initiative, produce a coherent, evidence-backed and reviewable engineering baseline from which a developer or coding agent can begin implementation with minimal requirement rediscovery.

### 3.2 Supporting goals

- Preserve the distinction between user-confirmed facts, external evidence, agent proposals, assumptions, and unknowns.
- Ask the user only questions whose answers materially affect value, scope, architecture, cost, or irreversible decisions.
- Adapt analysis depth and artifacts to project type, criticality, novelty, and risk.
- Maintain traceability from goals to scenarios, requirements, decisions, work slices, and verification.
- Make disagreement and uncertainty visible instead of manufacturing consensus.
- Allow requirements and decisions to evolve without silently invalidating downstream plans.
- Keep humans in control of product intent, major trade-offs, and acceptance of residual risk.

### 3.3 Non-goals

The first release MUST NOT:

- generate or modify production source code;
- autonomously deploy, purchase services, contact people, or change external systems;
- replace user ownership of business goals or irreversible decisions;
- promise that generated designs are correct merely because they are complete;
- require a particular development methodology such as Scrum or waterfall;
- generate every possible engineering document;
- provide unrestricted shell access to agents;
- support real-time multi-user editing;
- serve as a general chat assistant;
- hide important assumptions inside prose.

## 4. Target Users and Use Cases

### 4.1 Primary user

An individual developer or student who can implement software but wants structured help with product framing, requirements, architecture, risk analysis, and delivery planning.

### 4.2 Initial project modes

The system MUST support these modes:

| Mode | Description | Typical evidence |
|---|---|---|
| `greenfield` | A new product, tool, service, application, or library | user statements, research |
| `feature` | A new capability in an existing system | repository, current behavior, request |
| `refactor` | Internal change intended to preserve observable behavior | code, tests, architecture constraints |
| `spike` | Time-boxed investigation intended to reduce uncertainty | questions, experiments, findings |

Defect diagnosis, incident response, and project management are useful future modes but are outside the first release.

### 4.3 Representative user journeys

#### Journey A: greenfield idea

1. User creates a project and enters a vague idea.
2. System classifies the work and identifies high-impact unknowns.
3. User answers a small batch of prioritized questions.
4. Agents build scenarios, requirements, quality goals, risks, and candidate strategies.
5. User approves or changes significant decisions.
6. Independent review reports gaps or contradictions.
7. System produces a ready baseline and selected artifacts.

#### Journey B: feature in an existing repository

1. User creates a `feature` project and grants read-only access to a repository root.
2. Repository tools collect architecture, dependency, interface, data, and test evidence.
3. Agents connect requested behavior to current system constraints.
4. System produces an impact analysis, necessary decisions, migration or compatibility work, implementation slices, and verification.

#### Journey C: technical spike

1. User states an uncertain decision and a time budget.
2. System defines decision criteria and the minimum evidence required.
3. Research or bounded experiments are proposed and approved.
4. Findings are recorded as evidence.
5. System recommends an option, records remaining uncertainty, and explicitly stops without pretending to have designed the entire project.

### 4.4 Functional requirements

These IDs define the product being implemented and SHOULD be referenced by issues and tests:

| ID | Requirement |
|---|---|
| `FR-001` | The user can create, rename, archive, delete, and list projects. |
| `FR-002` | The user can initialize a project from vague natural-language input without completing a fixed questionnaire. |
| `FR-003` | The system classifies project mode, uncertainty, criticality, and active engineering concerns and allows the user to correct the classification. |
| `FR-004` | The system stores typed, versioned Project Model entities and relations with provenance, confidence, approval status, and freshness. |
| `FR-005` | The orchestrator advances, skips, or reopens workflow stages according to recorded gates and reasons. |
| `FR-006` | The system runs only the specialist roles relevant to the current stage and concerns. |
| `FR-007` | Agent changes are schema-validated, revision-aware proposals; agents cannot directly confirm consequential entities. |
| `FR-008` | The system prioritizes missing information and presents no more than three related user questions per batch. |
| `FR-009` | The user can answer, defer, or reject questions and inspect why each question matters. |
| `FR-010` | The user can compare options and approve or reject decisions against an exact Project Model revision. |
| `FR-011` | The system records and resolves review findings without silently discarding dissent. |
| `FR-012` | The system routes and renders project-specific artifact views from the canonical model. |
| `FR-013` | The user can inspect traceability from goals through scenarios, requirements, decisions, work slices, and verification. |
| `FR-014` | Material changes trigger impact analysis, freshness updates, and artifact invalidation. |
| `FR-015` | Agent runs are persisted, streamed to the UI, budgeted, cancellable, and auditable. |
| `FR-016` | The user can grant and revoke read-only access to one or more repository roots for applicable project modes. |
| `FR-017` | Repository facts are captured as locatable evidence without exposing ignored secrets or permitting writes. |
| `FR-018` | The system performs deterministic readiness checks and requires user approval before creating an immutable baseline. |
| `FR-019` | The user can export a baseline as canonical JSON and an implementation-oriented Markdown packet. |
| `FR-020` | The application restores durable project, workflow, job, and run state after a normal restart. |

### 4.5 Product quality requirements

| ID | Quality requirement |
|---|---|
| `QR-001 Integrity` | An invalid or interrupted model response cannot partially mutate the Project Model; accepted command sets are atomic. |
| `QR-002 Reproducibility` | Rendering the same baseline with the same renderer version produces byte-identical JSON and semantically identical Markdown, excluding explicitly declared volatile metadata. |
| `QR-003 Responsiveness` | Excluding LLM and external tool time, ordinary project queries and mutations complete within 250 ms at p95 for a local project containing 10,000 entities on the recorded CI benchmark environment. |
| `QR-004 Feedback` | A persisted run emits its first SSE state event within 500 ms, and the UI can reconstruct current state after reconnecting. |
| `QR-005 Cancellation` | After cancellation, no new proposal can be applied; cooperative model/tool operations receive cancellation within two seconds. |
| `QR-006 Accessibility` | Core create, question, decision, review, baseline, and export workflows meet WCAG 2.2 AA and are keyboard operable. |
| `QR-007 Security` | Repository tools cannot read outside authorized canonical roots, including through traversal or symlink escape, as demonstrated by automated tests. |
| `QR-008 Privacy` | Secrets and full prompt/source content are excluded from normal logs, exports, and telemetry. |
| `QR-009 Recoverability` | Restarting the server cannot lose accepted model revisions; expired running-job leases recover deterministically to queued or failed state. |
| `QR-010 Maintainability` | Domain and workflow tests run without network access or a real model provider, and domain packages have no dependency on transport, storage, UI, or provider SDKs. |

## 5. Design Principles

### 5.1 Knowledge before documents

Structured entities and relations are canonical. Rendered documents MUST be reproducible from a Project Model revision. Manual edits to generated documents MUST either be rejected or translated back into model changes through a controlled workflow.

### 5.2 Problem space before solution space

Agents MUST NOT select frameworks, databases, architectures, or interfaces before the relevant goals, scenarios, constraints, quality attributes, and risks are understood.

### 5.3 Risk determines depth

The system MUST spend more effort on high-impact, uncertain, and difficult-to-reverse concerns. Low-risk projects SHOULD remain lightweight.

### 5.4 Evidence over confidence language

Claims MUST cite user confirmation, repository evidence, external sources, experiments, or clearly marked inference. Fluent prose is not evidence.

### 5.5 Explicit uncertainty

Unknowns and assumptions are first-class entities. An unresolved high-severity unknown MUST prevent the project from becoming `ready` unless a human explicitly accepts the risk.

### 5.6 Human authority

Agents MAY propose. Only the user or an explicit policy can confirm product intent, approve architecturally significant decisions, accept high residual risk, or declare the baseline ready.

### 5.7 Independent criticism

Review agents SHOULD receive the artifact and its evidence, but not the original agent's private deliberation. Reviews MUST seek counterexamples and conflicts rather than rewrite everything stylistically.

### 5.8 Useful sufficiency

The target is enough design to make implementation responsible and directed, not maximum documentation. Generated output SHOULD leave implementation-level choices to the implementer when they do not affect contracts, quality, cost, or risk.

### 5.9 Incremental and reversible operation

Every agent write is a proposal applied through validated commands. Runs MUST be cancellable, resumable, budgeted, auditable, and idempotent.

## 6. Canonical Project Model

### 6.1 Project aggregate

A project is the aggregate root for all engineering knowledge and workflow state:

```json
{
  "id": "prj_01J...",
  "name": "Assignment planner",
  "raw_request": "Help students keep track of assignments",
  "mode": "greenfield",
  "output_language": "en",
  "stage": "FRAMING",
  "status": "active",
  "appetite": {
    "kind": "time",
    "value": 4,
    "unit": "weeks",
    "flexibility": "fixed_time_variable_scope"
  },
  "revision": 17,
  "created_at": "RFC3339 timestamp",
  "updated_at": "RFC3339 timestamp"
}
```

Project status is `active | ready | archived | deleted`. `ready` means the current revision has an approved baseline. Editing a ready project returns its working status to `active` without erasing earlier baselines. `appetite` is optional but SHOULD be established during framing because solution quality is relative to available time, cost, and scope.

### 6.2 Base entity

All domain objects use a common envelope:

```json
{
  "id": "req_01J...",
  "project_id": "prj_01J...",
  "kind": "requirement",
  "title": "A student can record an assignment",
  "body": {},
  "status": "proposed",
  "origin": "agent",
  "confidence": 0.78,
  "freshness": "current",
  "source_refs": ["evidence_01J..."],
  "tags": ["mvp"],
  "created_at": "RFC3339 timestamp",
  "updated_at": "RFC3339 timestamp",
  "revision": 3
}
```

`body` is validated against a schema selected by `kind`. Unknown fields MUST be rejected at command boundaries unless a schema explicitly allows extensions.

### 6.3 Shared enums

Entity status:

```text
draft | proposed | confirmed | rejected | superseded | unresolved
```

Origin:

```text
user | repository | external_source | experiment | agent | policy
```

Confidence is a value from `0.0` to `1.0`. It is advisory and MUST NOT override origin or approval status.

Freshness is independent of approval status:

```text
current | potentially_stale | stale
```

Impact analysis can therefore mark a confirmed entity as stale without pretending that its earlier approval never occurred.

Confirmation and provenance rules:

- Raw user messages and answers are stored as immutable `user_statement` evidence.
- An agent interpretation of a user statement remains `proposed` until the user confirms it or a narrow deterministic rule maps an explicit answer to the exact subject of a pending question.
- Repository and external evidence can confirm what a source says, but cannot by itself confirm a product goal, requirement, decision, or risk acceptance.
- Policy may confirm low-risk derived metadata such as project classification, but the user can correct it.
- Approvals are scoped to an entity version, decision version, risk version, or complete project revision. A material change invalidates the affected approval.
- `origin` identifies who or what created the assertion; `source_refs` identify the evidence supporting it. These fields are not interchangeable.

### 6.4 Entity kinds

#### `goal`

Required fields:

- `outcome`: desired change in the world or user behavior;
- `success_signals`: observable indicators;
- `time_horizon`: optional period in which value should appear;
- `priority`: `must | should | could`.

Goals describe outcomes, not features.

#### `stakeholder`

- `role`;
- `interests`;
- `authority`;
- `impact`;
- `contact_required`: boolean only; personal data is not required.

#### `context`

- `current_state`;
- `system_boundary`;
- `external_dependencies`;
- `baseline_behavior`;
- `project_mode`.

#### `scope_item`

- `statement`;
- `disposition`: `in_scope | out_of_scope | later`;
- `rationale`;
- `priority`: optional;
- `revisit_trigger`: optional.

Non-goals MUST be represented as `out_of_scope` items rather than being buried in a generated brief.

#### `constraint`

- `category`: `time | budget | technology | compatibility | legal | organizational | operational | physical`;
- `statement`;
- `hard`: boolean;
- `rationale`.

#### `assumption`

- `statement`;
- `impact_if_false`: `low | medium | high | critical`;
- `validation_method`;
- `expires_at`: optional;
- `owner`.

#### `question`

- `prompt`;
- `reason`;
- `answer_type`;
- `options`: optional;
- `impact`;
- `uncertainty`;
- `irreversibility`;
- `blocking`: boolean;
- `answer`: optional.

Question priority is calculated as:

```text
priority = impact * uncertainty * irreversibility
```

Each factor uses a normalized `1..5` scale. The product MAY later add effort and information availability to this formula.

#### `term`

- `name`;
- `definition`;
- `aliases`;
- `scope`;
- `source_ref`: optional.

Terms capture domain language whose ambiguity could affect scenarios, requirements, interfaces, or decisions. The system SHOULD reuse confirmed terms consistently in generated views.

#### `scenario`

- `actor`;
- `trigger`;
- `preconditions`;
- `main_flow`;
- `alternative_flows`;
- `failure_flows`;
- `postconditions`;
- `frequency`: optional;
- `importance`.

#### `requirement`

- `statement`;
- `category`: `functional | constraint | interface | data | operational`;
- `rationale`;
- `acceptance_conditions`;
- `priority`;
- `stability`: `stable | evolving | volatile`.

A confirmed requirement MUST be unambiguous enough to verify and MUST link to at least one goal or scenario.

Use a `constraint` entity for a condition imposed on the project or solution process, such as a mandated platform or deadline. Use a `requirement` with category `constraint` only when the resulting system itself has a verifiable obligation, such as supporting a browser version or data-retention period.

#### `quality_scenario`

Quality requirements use a scenario rather than an adjective:

- `characteristic`;
- `source`;
- `stimulus`;
- `environment`;
- `artifact`;
- `response`;
- `measure`.

Example: "When 500 concurrent users request the dashboard under normal operation, the API returns the 95th percentile response within 400 ms."

#### `risk`

- `category`: `value | usability | feasibility | architecture | security | privacy | delivery | operational | compliance`;
- `cause`;
- `event`;
- `impact`;
- `likelihood`;
- `severity`;
- `mitigation`;
- `evidence_needed`;
- `residual_risk`.

#### `option`

- `decision_topic`;
- `description`;
- `benefits`;
- `costs`;
- `risks`;
- `fit_to_constraints`;
- `evidence_refs`.

#### `decision`

- `question`;
- `selected_option_id`;
- `rationale`;
- `consequences`;
- `alternatives_considered`;
- `revisit_triggers`;
- `significance`: `local | cross_cutting | architectural`;
- `approval_required`.

Architectural decisions MUST link to affected requirements, quality scenarios, risks, or constraints.

#### `system_element`

- `element_type`: `person | software_system | container | component | interface | datastore | external_system`;
- `responsibilities`;
- `boundary`;
- `technology`: optional and only when decided;
- `lifecycle`;
- `trust_zone`: optional.

#### `work_slice`

- `outcome`;
- `included`;
- `excluded`;
- `dependencies`;
- `verification_refs`;
- `risk_reduction`;
- `completion_conditions`;
- `order_hint`.

Work slices SHOULD be vertical, independently demonstrable increments. They MUST NOT be decomposed into file-level coding tasks in the design phase.

#### `experiment`

- `hypothesis`;
- `decision_criteria`;
- `method`;
- `inputs`;
- `time_box`;
- `safety_constraints`;
- `expected_evidence`;
- `result_evidence_refs`;
- `conclusion`: optional.

An experiment describes a bounded spike, prototype, benchmark, or research activity. The planning workspace may specify it, but execution requires a separately authorized tool or a human in the first release.

#### `evidence`

- `evidence_type`: `user_statement | repository_fact | external_source | experiment_result | measurement`;
- `summary`;
- `locator`;
- `captured_at`;
- `freshness`;
- `trust_notes`.

#### `verification`

- `target_ref`;
- `method`: `test | review | analysis | demonstration | measurement | experiment`;
- `procedure`;
- `expected_result`;
- `environment`;
- `owner`.

### 6.5 Relations

Relations are typed directed edges:

```json
{
  "id": "rel_01J...",
  "project_id": "prj_01J...",
  "from_id": "req_...",
  "type": "satisfies",
  "to_id": "goal_...",
  "rationale": "...",
  "created_by": "agent_run_..."
}
```

Initial relation types:

```text
motivates | affects | constrains | assumes | answers | derives_from
satisfies | verifies | mitigates | selects | rejects | depends_on
conflicts_with | supersedes | implements | decomposes | evidenced_by
```

The service MUST validate relation compatibility. For example, a `verification` can `verify` a requirement, quality scenario, goal, decision consequence, or work slice, but not an arbitrary artifact version.

### 6.6 Project assessment

At intake and after material changes, the system calculates an assessment:

```json
{
  "mode": "feature",
  "system_types": ["web_application", "service"],
  "criticality": "medium",
  "novelty": 3,
  "domain_uncertainty": 2,
  "technical_uncertainty": 4,
  "change_scope": 3,
  "data_sensitivity": 4,
  "operational_exposure": 3,
  "active_concerns": ["interaction", "api", "data", "security", "migration"]
}
```

Scores use `1..5`. Assessment results select workflow activities and artifacts; they do not silently create requirements.

## 7. Adaptive Workflow

### 7.1 Workflow structure

The workflow is a state machine with reopenable stages, not a waterfall. Each stage has an entry condition, activities, readiness gate, and possible transition. Stages MAY be skipped only when the controller records why they are not applicable.

```text
INTAKE
  -> FRAMING
  -> CONTEXT
  -> SCENARIOS
  -> REQUIREMENTS
  -> SHAPING
  -> DECISIONS
  -> DELIVERY
  -> REVIEW
  -> READY
```

Any material new evidence can reopen an earlier stage. Reopening MUST mark downstream entities as `potentially_stale` until impact analysis clears or revises them.

### 7.2 Stage definitions

| Stage | Primary purpose | Readiness gate |
|---|---|---|
| `INTAKE` | Capture raw request and classify project | mode and initial assessment recorded |
| `FRAMING` | Establish outcomes, stakeholders, success, non-goals, appetite | at least one confirmed goal; boundary and non-goals understandable |
| `CONTEXT` | Understand baseline, constraints, dependencies, vocabulary | material constraints and external dependencies represented; evidence gaps visible |
| `SCENARIOS` | Describe behavior through normal, alternative, failure, and edge flows | priority user/system scenarios cover stated goals |
| `REQUIREMENTS` | Derive testable functional and quality expectations | confirmed requirements are traced and verifiable; important quality concerns addressed |
| `SHAPING` | Explore solution shape and reduce major unknowns without over-specifying | macro solution is coherent, bounded, and major feasibility risks have treatment |
| `DECISIONS` | Compare and approve significant trade-offs | architectural decisions have evidence, consequences, and approval state |
| `DELIVERY` | Define implementation slices and verification | slices cover approved scope; dependencies and completion evidence are defined |
| `REVIEW` | Independently test coherence and readiness | no unresolved blocking finding; residual high risks explicitly accepted |
| `READY` | Freeze an implementation baseline | user approves the baseline revision |

### 7.3 Stage transitions

The controller MUST evaluate gates using deterministic rules plus specialist recommendations. An LLM MUST NOT directly set a stage to `READY`.

Each transition emits an event containing:

- previous and next stage;
- gate checks and results;
- unresolved items;
- model revision;
- actor that requested the transition;
- approval reference when required.

### 7.4 Question policy

Before asking the user, the orchestrator MUST classify a missing fact:

1. `discoverable`: obtain it from authorized repository or research tools;
2. `decidable`: only a stakeholder can choose it;
3. `defaultable`: a low-impact, reversible assumption is acceptable;
4. `deferable`: it does not need resolution in the current stage.

Question behavior:

- Ask no more than three related high-priority questions in one batch.
- Explain briefly why each answer matters.
- Prefer concrete choices when genuine alternatives are known, while allowing free-form answers.
- Never ask the user to choose an implementation detail before presenting consequences.
- Do not repeat a question answered by evidence or a prior decision.
- Record unanswered non-blocking questions as assumptions or open questions.
- Escalate a question to blocking only when different answers cause materially different scope, architecture, compliance, or irreversible cost.

### 7.5 Risk-driven inquiry

For each high-severity risk, the orchestrator selects the cheapest adequate treatment:

```text
clarify -> inspect evidence -> research -> model -> prototype/spike
        -> decide -> accept -> avoid
```

The system MUST prefer reducing uncertainty over generating more prose. If a design claim cannot be responsibly established without an experiment, the output SHOULD be a spike plan and decision criteria, not a fabricated conclusion.

### 7.6 Change impact analysis

When a confirmed entity changes:

1. Traverse outgoing and incoming relations.
2. Mark affected proposed or confirmed entities `potentially_stale`.
3. Ask relevant specialist agents to classify each impact as `none | revise | invalidate`.
4. Present a change set to the user.
5. Apply approved changes atomically.
6. Regenerate affected artifact views.

## 8. Dynamic Artifact System

### 8.1 Principle

Artifacts are projections of a Project Model revision. They are not separate truth stores. Every generated section SHOULD link back to entity IDs, even if those IDs are hidden in normal reading mode.

### 8.2 Universal views

All projects expose these UI views, but they need not be separate files:

- purpose and scope;
- confirmed facts and assumptions;
- open questions;
- risks and decisions;
- traceability and readiness;
- delivery slices and verification when applicable.

### 8.3 Conditional view routing

| Triggered concern | Required view | Typical triggers |
|---|---|---|
| `interaction` | user flow and UI state model | human-facing interface |
| `api` | interface contract | network boundary, plugin boundary, public module API |
| `data` | conceptual data model | persistent or exchanged domain data |
| `migration` | migration and rollback strategy | existing data, interface, or behavior changes |
| `runtime` | runtime interaction view | concurrency, asynchronous work, complex collaboration |
| `deployment` | deployment view | multiple processes, environments, infrastructure dependencies |
| `security` | threat and trust-boundary analysis | identity, network exposure, sensitive operations |
| `privacy` | data inventory and privacy treatment | personal or sensitive data |
| `reliability` | failure model and recovery strategy | availability or durability expectations |
| `performance` | performance model and measurement plan | explicit latency, throughput, scale, or resource constraints |
| `compatibility` | version and compatibility strategy | library, SDK, public API, protocol, file format |
| `repository_change` | current-state and impact analysis | feature or refactor in existing repository |
| `experiment` | hypothesis and evidence report | spike or unresolved feasibility question |

Routers use assessment scores, entity presence, risk severity, and explicit user requests. Users MAY enable or disable a non-mandatory view. Mandatory views require a recorded risk acceptance before being disabled.

### 8.4 Supported render targets

Initial release:

- in-app HTML;
- Markdown export;
- JSON export of the canonical Project Model;
- Mermaid diagrams for supported system and flow views.

PDF, DOCX, Jira, GitHub Issues, and external wiki export are deferred.

### 8.5 Artifact versioning

Every artifact version stores:

- renderer type and version;
- source Project Model revision;
- included entity IDs;
- generation timestamp;
- checksum;
- generation run ID.

Artifacts MUST display a stale warning when their source model revision is no longer current and affected entities changed.

### 8.6 Implementation handoff packet

Markdown export for a coding agent is a dynamically composed packet, not a dump of every view. It MUST include:

1. baseline identity, revision, checksum, and generation time;
2. confirmed purpose, success conditions, scope, and non-goals;
3. applicable context, constraints, glossary, and repository evidence;
4. scenarios and confirmed requirements;
5. applicable quality scenarios and risk treatments;
6. approved decisions with rationale and consequences;
7. routed system, interaction, interface, data, security, runtime, or migration views;
8. ordered work slices with dependencies and completion conditions;
9. verification obligations and definition of readiness for implementation completion;
10. accepted residual risks, assumptions, and unresolved non-blocking questions;
11. explicit instructions not to reinterpret rejected options or silently resolve open items.

Each section MUST preserve stable entity IDs in anchors or adjacent metadata so a coding agent can report progress and questions against the original design. Empty or non-applicable sections MUST be omitted.

## 9. Multi-Agent Design

### 9.1 Orchestrator

The Orchestrator is deterministic application code. It owns:

- current stage and gate evaluation;
- project assessment and concern routing;
- specialist selection;
- context-pack construction;
- run budgets, retries, cancellation, and timeouts;
- proposal validation and transactional application;
- approval requests;
- audit events;
- artifact invalidation and regeneration.

It MUST NOT be implemented as a single unconstrained LLM prompt.

### 9.2 Specialist roles

| Role | Responsibility | Primary outputs |
|---|---|---|
| `discovery` | clarify outcome, stakeholders, baseline, scope, constraints | goals, stakeholders, context, questions, assumptions |
| `requirements` | derive scenarios and verifiable requirements | scenarios, requirements, quality scenarios, glossary terms |
| `architecture` | shape boundaries, responsibilities, options, and significant decisions | system elements, options, decisions, technical risks |
| `quality_risk` | challenge quality, security, privacy, operations, and feasibility | risks, quality scenarios, mitigations, spike proposals |
| `delivery` | create vertical implementation slices and verification strategy | work slices, dependencies, verification |
| `critic` | independently find contradictions, omissions, weak evidence, and over-design | review findings only |

Not every role runs in every stage. Security or domain-specific specialist prompts MAY be added later as routed capabilities rather than permanent participants.

### 9.3 Collaboration protocol

Agents MUST communicate through typed proposals, never by directly editing shared prose.

```json
{
  "run_id": "run_...",
  "base_revision": 17,
  "summary": "Derived assignment creation scenarios",
  "commands": [
    {"type": "create_entity", "entity": {}},
    {"type": "create_relation", "relation": {}},
    {"type": "request_approval", "subject_ref": "decision_..."}
  ],
  "warnings": [],
  "unresolved": [],
  "recommended_next_action": "run_requirements_review"
}
```

The command validator MUST check schemas, references, permissions, base revision, duplicate semantics, and role authority. Conflicting concurrent proposals are not auto-merged; the orchestrator requests reconciliation or user review.

### 9.4 Agent execution patterns

Use only these patterns in the first release:

1. **Sequential refinement**: one specialist builds on confirmed upstream entities.
2. **Independent review**: critic receives a clean context pack and reports findings.
3. **Parallel option generation**: at most three isolated runs propose alternatives against identical criteria.
4. **Synthesis**: architecture agent compares already visible options; it MUST preserve minority risks.

Free-form round-table discussion is explicitly prohibited because it is expensive, difficult to audit, and prone to false consensus.

### 9.5 Prompt architecture

Prompts are assembled in layers:

```text
platform safety policy
-> shared engineering policy
-> role contract
-> stage objective and readiness gate
-> project context pack
-> exact task
-> output JSON schema
```

Shared engineering policy MUST include:

- distinguish fact, evidence, inference, proposal, assumption, and decision;
- do not invent user preferences or repository facts;
- avoid solution choices before relevant requirements and constraints;
- create measurable conditions where measurement matters;
- cite entity or evidence IDs for claims;
- report uncertainty and conflicts;
- minimize irrelevant artifacts and premature implementation detail;
- output only the requested structured response;
- never claim human approval.

Prompts SHOULD request concise rationale and evidence references. They MUST NOT request or expose hidden chain-of-thought. The application displays actions, evidence, decisions, and short summaries instead.

### 9.6 Context packs

Each run receives the minimum sufficient context:

- project summary and current stage;
- relevant confirmed entities;
- relevant proposed entities when the task is review or reconciliation;
- relation neighborhood;
- active risks and questions;
- applicable constraints and decisions;
- source excerpts with stable locators;
- gate criteria;
- token and tool budgets.

The context builder MUST use entity selection and summaries rather than replaying the entire chat history. User messages are retained as evidence but are not the primary working memory.

### 9.7 Model routing

The core must be provider-neutral:

```go
type ModelClient interface {
    Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
    Stream(ctx context.Context, req GenerateRequest) (<-chan StreamEvent, error)
}
```

`GenerateRequest` includes messages, tool schemas, response schema, temperature policy, token budget, and provider-independent metadata. Provider-specific configuration remains inside adapters.

The first implementation MAY support one provider, but domain and orchestration packages MUST NOT import its SDK directly.

### 9.8 Run controls

Every run MUST define:

- maximum model turns;
- maximum tool calls;
- input and output token budget;
- wall-clock timeout;
- cancellation context;
- retry policy;
- allowed tools;
- base model revision;
- idempotency key.

Retry only transient provider failures, rate limits, and schema-repair attempts. Do not retry a semantically rejected proposal without changing instructions or context.

### 9.9 Orchestration loop

```text
receive user command or workflow continuation
-> load project revision and evaluate deterministic gates
-> choose the next bounded task and authorized specialist
-> build the minimum context pack
-> persist run and budget before contacting the model
-> execute model/tool turns under policy
-> validate the final typed proposal
-> if base revision changed, stop for reconciliation
-> atomically apply AUTO_PROPOSE commands as proposed entities
-> create approval requests for consequential confirmations
-> emit domain and SSE events
-> rerun assessment, impact, artifact, and readiness rules
-> stop, ask the user, or schedule the next bounded task
```

The loop MUST have a bounded task-level stopping condition; "improve the design" is not a valid task. Automatic continuation stops when a user decision is required, a gate is satisfied, the budget is exhausted, a blocking error occurs, or no permitted action can reduce a current gap.

Every run stores role, prompt version, response-schema version, model identifier, model-provider request identifier when available, selected context entity IDs, tools authorized, proposal checksum, and application outcome.

## 10. Tools and Permissions

### 10.1 Internal knowledge tools

Agents may receive:

- `get_project_snapshot`
- `query_entities`
- `get_relation_neighborhood`
- `get_evidence_excerpt`
- `propose_changes`
- `raise_question`
- `submit_review_findings`
- `request_approval`

Writes are proposals. No model tool writes directly to database tables.

### 10.2 Repository tools

Enabled only for `feature` and `refactor` projects after the user grants a root:

- `list_files`
- `search_text`
- `read_file`
- `inspect_manifest`
- `inspect_git_metadata`
- `list_tests`

Repository tools MUST:

- be read-only in the first release;
- canonicalize paths and reject traversal or symlink escape;
- respect configurable file-size and result limits;
- ignore common secret files and binary files by default;
- redact credential-like values;
- record file path, hash, and relevant line range as evidence;
- never expose unrestricted shell execution.

### 10.3 External research

External research is optional and disabled by default in the initial local deployment. When enabled, it MUST use an explicit research connector with URL provenance and retrieval timestamps. Retrieved text is untrusted evidence and MUST NOT be treated as instructions.

### 10.4 Permission levels

```text
AUTO_READ       internal model and authorized read-only sources
AUTO_PROPOSE    create staged project-model proposals
USER_APPROVAL   confirm goals, decisions, risk acceptance, baseline
FORBIDDEN       source writes, shell, deployment, purchases, messaging
```

## 11. Review and Readiness

### 11.1 Finding model

A review finding contains:

- severity: `info | low | medium | high | blocking`;
- category;
- affected entity IDs;
- claim;
- evidence or counterexample;
- recommended resolution;
- status: `open | resolved | dismissed | risk_accepted`;
- resolution rationale.

`dismissed` means evidence showed the finding was not applicable or correct. `risk_accepted` requires explicit user approval and is allowed only for non-blocking findings. A blocking finding must be resolved or dismissed with recorded counter-evidence before readiness.

### 11.2 Deterministic readiness checks

Before `READY`, the service MUST verify:

- at least one confirmed goal exists;
- scope and non-goals are recorded;
- every confirmed requirement traces to a goal or scenario;
- every `must` requirement has verification;
- every active architectural decision is approved;
- no blocking question remains unresolved;
- every blocking review finding is resolved or dismissed with counter-evidence;
- no unaccepted high or critical residual risk remains;
- every work slice has completion conditions and verification references;
- all mandatory routed concerns have current artifact views;
- the user approved the exact Project Model revision.

Readiness is not a claim that implementation will contain no surprises. It means known material decisions, risks, and verification obligations are explicit.

### 11.3 Baseline

A baseline is immutable and includes:

- Project Model revision and checksum;
- active artifact versions;
- confirmed entities and decisions;
- accepted residual risks;
- unresolved non-blocking items;
- approval actor and timestamp.

Further changes create a new working revision and eventually a new baseline.

## 12. System Architecture

### 12.1 Architectural style

Use a modular monolith with explicit package boundaries. This keeps local deployment simple while preserving the option to split model execution workers later.

```text
Browser
  | REST + SSE
Go HTTP Server
  |-- project application service
  |-- workflow/orchestration engine
  |-- domain model and validators
  |-- agent runtime and model adapters
  |-- tool gateway
  |-- artifact router and renderers
  |-- review/readiness engine
  |-- persisted job runner
  `-- SQLite repositories and event log
```

### 12.2 Backend technology decisions

- Language: current stable Go version at repository creation; pin in `go.mod` and CI.
- HTTP router: `chi` or Go standard library routing. Prefer `chi` only if middleware ergonomics justify it.
- Database: SQLite through a pure-Go driver for portable local builds.
- Migrations: versioned SQL files embedded into the binary.
- API encoding: JSON.
- Live updates: SSE. WebSockets are unnecessary until bidirectional collaborative sessions exist.
- Logging: structured `slog` with sensitive-field redaction.
- IDs: UUIDv7 or ULID; choose one implementation and use sortable opaque IDs consistently.
- Configuration: environment variables plus optional local config file; secrets never stored in project export.
- Background work: persisted in-process job queue with leases. Redis or external brokers are deferred.

### 12.3 Frontend technology decisions

- React with TypeScript and Vite.
- React Router for application routes.
- TanStack Query for server state and mutation invalidation.
- A small local UI-state store only when React state is insufficient; do not duplicate server state.
- Accessible component primitives; keyboard operation and visible focus are required.
- Markdown rendering with sanitization.
- Mermaid rendering in a sandboxed or sanitized path.
- No marketing landing page. The first screen is the project workspace.

### 12.4 Backend package layout

```text
cmd/server/                 process entrypoint
internal/domain/            entities, relations, policies, validation
internal/application/       commands, queries, transactions
internal/workflow/          stages, gates, impact analysis
internal/orchestrator/      run planning and specialist routing
internal/agents/            role contracts and prompt assembly
internal/models/            provider-neutral interfaces and adapters
internal/tools/             internal, repository, and research gateways
internal/artifacts/         concern router and renderers
internal/review/            findings and readiness checks
internal/jobs/              persisted queue, leases, retries
internal/storage/sqlite/    repositories and migrations
internal/httpapi/           REST handlers, SSE, middleware
internal/audit/             immutable domain and security events
web/                        React application
prompts/                    versioned prompt templates and schemas
testdata/evals/             multi-domain evaluation fixtures
```

Domain packages MUST NOT depend on HTTP, SQLite, React, or a model provider.

## 13. Persistence Model

Use normalized identity and relation tables with JSON payloads for kind-specific bodies.

Core tables:

```text
projects
project_revisions
entities
entity_versions
relations
assessments
workflow_states
approvals
review_findings
agent_runs
agent_run_steps
tool_calls
jobs
events
artifacts
artifact_versions
repository_grants
```

Questions are canonical `question` entities and MUST NOT also be stored as independent mutable facts. If a dedicated question lookup table is introduced for performance, it is a rebuildable projection keyed to entity version. The same rule applies to any future entity-specific index.

Important constraints:

- entity IDs are stable; updates create `entity_versions`;
- project revision increments once per accepted transaction;
- relation endpoints must belong to the same project;
- approvals reference exact subject and revision;
- event records are append-only;
- agent proposals include `base_revision` for optimistic concurrency;
- deleting a domain entity means superseding or tombstoning it, not erasing audit history.

SQLite SHOULD use WAL mode, foreign keys, busy timeouts, and short write transactions. Agent calls MUST occur outside database transactions.

## 14. HTTP API

All endpoints are under `/api/v1`. Error responses use a stable problem-details shape with `code`, `message`, `details`, and `request_id`.

### 14.1 Projects

```text
POST   /projects
GET    /projects
GET    /projects/{projectID}
PATCH  /projects/{projectID}
POST   /projects/{projectID}/archive
GET    /projects/{projectID}/snapshot
GET    /projects/{projectID}/revisions
```

### 14.2 Model commands and queries

```text
GET    /projects/{projectID}/entities
GET    /projects/{projectID}/entities/{entityID}
POST   /projects/{projectID}/commands
GET    /projects/{projectID}/relations
GET    /projects/{projectID}/traceability
GET    /projects/{projectID}/impact?entity_id=...
```

The generic command endpoint accepts validated command envelopes and an `expected_revision`.

### 14.3 Workflow and agents

```text
GET    /projects/{projectID}/workflow
POST   /projects/{projectID}/workflow/continue
POST   /projects/{projectID}/workflow/reopen
POST   /projects/{projectID}/runs
GET    /projects/{projectID}/runs
GET    /projects/{projectID}/runs/{runID}
POST   /projects/{projectID}/runs/{runID}/cancel
GET    /projects/{projectID}/events
```

`GET /events` is an SSE stream supporting `Last-Event-ID` recovery.

Initial event types:

```text
project.revision_created
workflow.stage_changed
workflow.blocked
question.created
question.answered
approval.requested
approval.resolved
run.queued
run.state_changed
run.tool_started
run.tool_finished
run.proposal_validated
run.completed
run.failed
run.cancelled
review.finding_created
artifact.rendered
artifact.invalidated
baseline.created
```

Every SSE message uses the durable event sequence as `id`, includes `project_id`, `occurred_at`, and a versioned payload, and can be safely replayed by the frontend.

### 14.4 Questions and approvals

```text
GET    /projects/{projectID}/questions
POST   /projects/{projectID}/questions/{questionID}/answer
GET    /projects/{projectID}/approvals
POST   /projects/{projectID}/approvals/{approvalID}/approve
POST   /projects/{projectID}/approvals/{approvalID}/reject
```

Approval mutations MUST require the expected project revision and an optional rationale.

### 14.5 Reviews and artifacts

```text
GET    /projects/{projectID}/reviews
POST   /projects/{projectID}/reviews/{findingID}/resolve
GET    /projects/{projectID}/artifacts
POST   /projects/{projectID}/artifacts/render
GET    /projects/{projectID}/artifacts/{artifactID}
GET    /projects/{projectID}/export?format=markdown|json
POST   /projects/{projectID}/baseline
```

### 14.6 Idempotency

All command endpoints SHOULD accept `Idempotency-Key`. The server stores the key, request checksum, and response for a bounded period. Reuse with a different body returns a conflict.

## 15. Web Experience

### 15.1 Information architecture

```text
/projects                         project list and create action
/projects/:id/overview            stage, readiness, goals, risks, recent changes
/projects/:id/questions           prioritized question inbox
/projects/:id/decisions           option comparison and approvals
/projects/:id/model               entities, relations, filters, traceability
/projects/:id/artifacts           rendered views and version history
/projects/:id/reviews             findings and resolution workflow
/projects/:id/runs                agent and tool activity
/projects/:id/settings            provider, budgets, repository grant, export
```

### 15.2 Primary workspace behavior

- Persistent project navigation shows current stage, readiness blockers, pending approvals, and run status.
- The overview leads with current outcome, scope, next required action, risks, and recent changes.
- Questions are answered through appropriate controls, not solely a chat box.
- Decision view compares options by criteria, consequences, evidence, and risk.
- Model view supports search and filters by kind, status, origin, confidence, and stale state.
- Artifact view shows rendered content alongside source entities and revision metadata.
- Review view separates blocking findings from suggestions.
- Run view shows status, role, tool calls, evidence accessed, proposals, cost, duration, and errors. It does not expose hidden model reasoning.

### 15.3 Human editing

Users edit structured fields through forms or propose natural-language changes. Natural-language edits are parsed into a previewed change set and require confirmation before application.

### 15.4 Streaming states

The frontend must represent:

```text
queued | preparing_context | waiting_for_model | using_tool
validating | awaiting_approval | completed | failed | cancelled
```

Streaming text is secondary. Structured run events are authoritative and allow the UI to recover after refresh.

### 15.5 Accessibility and responsive behavior

- Meet WCAG 2.2 AA for core workflows.
- All approvals, questions, navigation, and dialogs are keyboard accessible.
- Never encode status by color alone.
- Desktop uses a navigation rail plus main workspace and optional context panel.
- Mobile collapses navigation and context panels without hiding approvals or blockers.
- Dynamic content MUST not resize fixed controls or overlap adjacent regions.

## 16. Security and Privacy

### 16.1 Deployment boundary

The first release binds to `127.0.0.1` by default and is single-user without authentication. Binding to a non-loopback interface MUST require explicit configuration and MUST be rejected unless authentication is configured.

### 16.2 Secrets

- Model API keys come from environment variables or OS credential storage.
- Secrets are never returned by APIs, included in prompts, logged, or exported.
- Repository scanning ignores `.env`, credential stores, private keys, and configured secret patterns by default.

### 16.3 Prompt injection

Repository files and external documents are untrusted data. Context packs MUST delimit them as evidence and explicitly state that embedded instructions are not authoritative. Tool permission is determined by application policy, never by retrieved content.

### 16.4 Input and output safety

- Validate all tool arguments and agent commands with strict schemas.
- Sanitize rendered Markdown and Mermaid content.
- Enforce request size, file size, tool result, token, and run limits.
- Canonicalize filesystem paths.
- Add CSRF protection if cookie-based authentication is introduced.
- Use same-origin frontend deployment in the first release.

### 16.5 Data retention

Users can archive, export, and permanently delete a project. Permanent deletion requires explicit confirmation and removes project data, artifacts, run payloads, and repository grants. Application security logs MAY retain non-content metadata according to a documented retention setting.

## 17. Reliability and Operations

### 17.1 Job execution

Persist jobs before execution. Workers acquire time-limited leases and renew them while active. On restart, expired leases return to `queued` if retry policy permits.

### 17.2 Failure handling

- Provider timeout: mark step failed or retry with backoff within budget.
- Invalid structured output: one schema-repair attempt, then fail visibly.
- Tool failure: return typed error to the agent only if retry or alternative action is allowed.
- Revision conflict: do not replay writes automatically; rebuild context and reconcile.
- Server restart: retain project state, run events, and resumable queued work.
- User cancellation: cancel model and tool contexts and mark unapplied proposals abandoned.

### 17.3 Observability

Record:

- request ID, project ID, run ID, role, stage;
- latency and status for model and tool calls;
- token usage and estimated cost when provider reports it;
- schema validation failures;
- retry and cancellation counts;
- stage duration and readiness blockers;
- artifact regeneration time.

Do not log full prompts, user content, source excerpts, or model responses by default. A local debug mode may retain them with an explicit privacy warning.

### 17.4 Backups and export

The application SHOULD support consistent SQLite backup and complete JSON project export. Import is deferred until the export schema is versioned and migration behavior is defined.

## 18. Quality Strategy

### 18.1 Testing layers

#### Unit tests

- entity and relation validation;
- readiness gates;
- question priority;
- concern routing;
- change impact traversal;
- prompt context selection;
- command authorization;
- path sandboxing;
- artifact staleness.

#### Integration tests

- SQLite transactions and migrations;
- persisted jobs and lease recovery;
- model adapter with a deterministic fake provider;
- schema repair behavior;
- command idempotency and revision conflicts;
- REST and SSE recovery;
- export reproducibility.

#### End-to-end tests

- create a project from vague input;
- answer questions;
- continue through workflow;
- approve a decision;
- resolve a review finding;
- create a baseline;
- refresh during a run and recover state;
- cancel a run;
- export Markdown and JSON.

### 18.2 Agent evaluations

Use versioned fixtures and a deterministic evaluation harness. Do not grade based on document length or stylistic similarity.

Core metrics:

```text
goal_traceability       goals connected to scenarios/requirements
verification_coverage   must requirements with verification
evidence_coverage       material claims with evidence or explicit assumption
contradiction_count     incompatible active entities
question_efficiency     decision-changing answers / questions asked
risk_recall             seeded material risks identified
irrelevant_artifacts    views generated without a triggered concern
decision_quality        alternatives, criteria, consequences, revisit triggers
change_consistency      affected downstream entities correctly invalidated
human_rework            major clarifications required after READY
```

Evaluation fixtures MUST include at least:

1. a small local CLI with no network or database;
2. an existing web application feature involving authentication and stored data;
3. a public Go library API change with compatibility requirements;
4. a technical spike comparing two storage approaches;
5. an intentionally contradictory request;
6. a request containing prompt injection inside repository documentation.

### 18.3 Golden invariants

- The CLI fixture does not produce deployment, privacy, or database views without evidence that they apply.
- The authentication feature produces security, data, migration, and failure concerns.
- The library change produces compatibility and public contract analysis.
- The spike stops at findings and decision criteria instead of inventing a full product plan.
- Contradictions remain visible until resolved.
- Retrieved repository text cannot grant tools or change system policy.

## 19. Implementation Plan

Each milestone must be a working vertical slice. Do not build all prompts before the domain model and run controls exist.

### Milestone 0: repository foundation

- Go module and frontend workspace;
- formatting, linting, tests, and CI;
- embedded migrations;
- local configuration and structured logging;
- one-command development startup.

Exit: empty application starts, health check succeeds, frontend loads, migrations run in a temporary database.

### Milestone 1: Project Model core

- projects, revisions, entities, relations, events;
- schema validation and optimistic concurrency;
- project, model, and traceability APIs;
- basic project/model web views;
- JSON export.

Exit: user can manually create a coherent structured project and inspect its relation graph without an LLM.

### Milestone 2: single-agent framing slice

- model provider interface and one adapter;
- persisted runs, SSE, cancellation, budgets;
- discovery role and structured proposals;
- question inbox and answer flow;
- `INTAKE`, `FRAMING`, and `CONTEXT` gates.

Exit: vague input becomes goals, context, constraints, assumptions, and prioritized questions through auditable proposals.

### Milestone 3: adaptive engineering workflow

- requirements, architecture, quality/risk, and delivery roles;
- assessment and concern routing;
- stages through `DELIVERY`;
- option and approval workflow;
- change impact analysis.

Exit: all evaluation fixtures can reach `REVIEW` with different routed concerns.

### Milestone 4: independent review and readiness

- isolated critic context;
- review findings and resolution;
- deterministic readiness checks;
- baseline creation;
- readiness and blocker UI.

Exit: no fixture can reach `READY` with a seeded blocking contradiction or unapproved architectural decision.

### Milestone 5: dynamic artifacts

- artifact router;
- HTML, Markdown, JSON, and Mermaid renderers;
- artifact provenance, versions, and stale state;
- artifact workspace and export.

Exit: fixtures produce only relevant views, and artifacts are reproducible from a baseline.

### Milestone 6: existing repository support

- repository grants and safe read-only tools;
- evidence locators and hashes;
- impact analysis for `feature` and `refactor` modes;
- injection and path-escape security tests.

Exit: a feature fixture can cite repository evidence without exposing ignored secrets or leaving the authorized root.

## 20. Release Acceptance Criteria

Version 1 is acceptable when:

- a user can start with one paragraph of vague input and reach a reviewed baseline through the web interface;
- the system asks prioritized questions and records low-impact defaults as assumptions;
- different project fixtures produce materially different, relevant artifacts;
- goals, requirements, decisions, work slices, and verification are traceable;
- significant agent claims link to evidence or are explicitly marked as inference or assumption;
- user approval is required for major decisions, high residual risk, and the final baseline;
- agent runs survive refresh, can be cancelled, and obey budgets;
- model-provider failure cannot corrupt the Project Model;
- new evidence invalidates affected downstream entities and artifacts;
- the exported Markdown is sufficient for a separate coding agent to begin implementation while preserving entity IDs and unresolved items;
- all security and golden-invariant tests pass.

## 21. Deferred Capabilities

The following are intentionally deferred until the core workflow is validated:

- coding-agent handoff protocol beyond Markdown and JSON export;
- source-code modification;
- multi-user organizations, comments, and real-time collaboration;
- external project trackers and repository issue creation;
- custom role/plugin marketplace;
- domain packs for regulated industries;
- automatic prototype execution;
- semantic vector search;
- remote worker pools;
- mobile-specific application;
- PDF and office document export;
- autonomous lifecycle monitoring after implementation begins.

## 22. Key Risks and Mitigations

| Risk | Consequence | Mitigation |
|---|---|---|
| Fluent but unsupported output | false confidence | provenance, typed assumptions, evidence requirements, critic |
| Excessive questioning | user abandonment | information-value priority, batches of three, defaultable assumptions |
| Fixed-template behavior reappears | irrelevant documents | canonical model plus concern router and irrelevant-artifact eval |
| Agents converge on same mistake | false consensus | isolated option generation, independent critic, preserve dissent |
| Huge context and cost | slow, expensive runs | minimal context packs, budgets, summaries, entity queries |
| Premature architecture | requirements shaped around favorite tools | stage authority, problem-space gates, decision criteria |
| Stale documents | implementation follows invalid decisions | revision provenance, impact analysis, stale banners |
| Provider lock-in | expensive rewrite | provider-neutral interface and prompt schemas |
| Repository prompt injection | policy or data compromise | untrusted evidence boundary, fixed tool policy, security tests |
| Over-engineered first release | project never becomes usable | modular monolith, vertical milestones, deferred capability list |

## 23. Defaults and Configuration

The following defaults are design decisions, not unresolved blockers:

- local single-user deployment;
- one model provider configured at a time;
- SQLite and in-process jobs;
- REST and SSE;
- React/TypeScript frontend;
- Markdown and JSON export;
- English internal IDs and schemas; UI and generated prose may use the user's language;
- repository access is read-only;
- external research is opt-in;
- no unrestricted shell;
- no code generation.

Configurable per project:

- output language;
- model and budget policy;
- project mode and criticality;
- time or scope appetite;
- authorized repository root;
- enabled optional concerns;
- artifact rendering preferences;
- data retention and debug logging.

## 24. Reference Basis

This design synthesizes, rather than implements verbatim, the following sources:

- ISO/IEC/IEEE 12207:2026, software life cycle processes: a common framework that supports concurrent, iterative, recursive, and incremental application without mandating a specific lifecycle model or documentation format.  
  <https://www.iso.org/standard/90219.html>
- ISO/IEC/IEEE 29148:2018, requirements engineering: requirements processes and information items intended to apply across project scope, methodology, size, and complexity.  
  <https://www.iso.org/standard/72089.html>
- ISO/IEC 25010:2023, product quality model: quality characteristics used here as a concern-discovery reference rather than a mandatory checklist.  
  <https://www.iso.org/standard/78176.html>
- NIST SP 800-218, Secure Software Development Framework: security practices should be integrated into the chosen lifecycle.  
  <https://csrc.nist.gov/pubs/sp/800/218/final>
- Principles behind the Agile Manifesto: early value, changing requirements, technical excellence, simplicity, and working software as evidence.  
  <https://agilemanifesto.org/principles.html>
- Scrum Guide: transparency, inspection, adaptation, goals, increments, and an explicit definition of done. This product uses those empirical principles without requiring Scrum.  
  <https://scrumguides.org/scrum-guide.html>
- Shape Up: shaped work should be rough, solved at the macro level, bounded, and de-risked without specifying implementation details prematurely.  
  <https://basecamp.com/shapeup/1.1-chapter-02>
- arc42: context, constraints, decisions, quality, and risks as architecture concerns, with lean and thorough variants.  
  <https://docs.arc42.org/home/>
- C4 model: hierarchical, notation-independent architecture views selected according to the audience and question.  
  <https://c4model.com/>
- Architectural Decision Records: decisions preserve rationale, alternatives, trade-offs, and consequences.  
  <https://adr.github.io/>

Recommended deeper reading for implementers designing prompt policies and evaluation fixtures:

- *Software Requirements*, Karl Wiegers and Joy Beatty;
- *Mastering the Requirements Process*, Suzanne Robertson and James Robertson;
- *Software Architecture in Practice*, Len Bass, Paul Clements, and Rick Kazman;
- *Designing Software Architectures: A Practical Approach*, Humberto Cervantes and Rick Kazman;
- *Domain-Driven Design*, Eric Evans;
- *Continuous Delivery*, Jez Humble and David Farley;
- *Accelerate*, Nicole Forsgren, Jez Humble, and Gene Kim;
- *Guide to the Software Engineering Body of Knowledge (SWEBOK Guide)*, IEEE Computer Society.

## 25. Final Implementation Rule

When an implementation choice is not specified here, choose the simplest option that preserves:

1. canonical structured project knowledge;
2. explicit provenance and uncertainty;
3. deterministic workflow authority;
4. human approval of consequential decisions;
5. risk-driven artifact selection;
6. auditability, cancellation, and reproducibility;
7. separation between domain logic and model-provider behavior.

Do not add complexity merely to imitate an organization chart or make the system appear more agentic. The product succeeds when it helps a developer make better, traceable engineering decisions with less avoidable uncertainty.
