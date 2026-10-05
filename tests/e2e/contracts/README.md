# Executable journey expectations

These small JSON contracts freeze the business observations before execution.
They are authored from the verification specification, never generated from the
candidate's response. Test code owns the sequence and independent invariants.

| Journey | Steps and intent | Boundaries |
| --- | --- | --- |
| `review-roundtrip` / VER-review | R1 register format; R2 publish through MCP; R3 person answers Local and instructions; R4 restart; R5 fresh MCP discovers and reads; R6 person corrects to Remote and immutable history remains | Synthetic consumer, no model judgment |
| `setup-no-run` / VER-setup | S1 save preferences/context; S2 CLI creates Interest/Watcher; S3 portal and MCP inspect without starting a Run | No scheduler or source access |
| `inspection-two-cycles` / VER-inspection | I1 CLI publishes comparable and incomplete findings; I2 portal saves note/Todo/reminder; I3 CLI reconciles completed source; I4 unchanged replay retains versions | Local cost fixtures, not economic/source accuracy |
| `recovery-answer` / VER-recovery | A1 lose committed answer response; A2 retry exact request; A3 refuse stale correction and retain draft; A4 correct against refreshed material | One named network fault and one fencing conflict |
| `recovery-run` / VER-recovery | U1 claim inspection; U2 restart and fresh MCP inspects; U3 person abandons; U4 another claim succeeds | No external agent launched |

Each run writes `report.md`, `actual.json`, screenshots, and owned-process logs.
Reference a case/step ID when giving feedback. Reports retain a failed or unreached
step. See `scripts/testing/README.md` for selection and proposal commands.

These are executable contract oracles, not accepted visual golden baselines.
Initial golden proposals go only to ignored output. Human acceptance of the
concrete proposal is required before adopting a committed golden; automated
passes do not establish that acceptance. Normal runs never rewrite expectations.
