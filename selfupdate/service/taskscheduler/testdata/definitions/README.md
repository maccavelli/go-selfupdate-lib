# Task definitions as registered and as Task Scheduler stored them

Each case is a pair:

- `<case>.wanted.xml` is the definition as it was registered, with
  `ITaskFolder::RegisterTask` (`TASK_CREATE_OR_UPDATE |
  TASK_IGNORE_REGISTRATION_TRIGGERS`, no user, `InteractiveToken`);
- `<case>.stored.xml` is what `IRegisteredTask::get_Xml` returned for it.

They were captured on the Windows test host (Windows 11, build 26200) on
2026-10-23, from a driver task at medium integrity in the console
session, by probe P1 step 7 of
[0032-PLAN](../../../../../docs/decisions/0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md).
Two runs captured identical XML.

Edits made before they were committed:

- the account's SID is the placeholder
  `S-1-5-21-1111111111-2222222222-3333333333-1001`, and its name
  `EXAMPLE\user`: real identifiers do not belong in the tree, and an
  angle-bracket placeholder would not be valid XML;
- the probe's random task-name suffix is `00000000`;
- Task Scheduler's CRLF line endings are LF, as `.gitattributes` keeps
  every text file.

The XML declaration says `UTF-16`, as Task Scheduler wrote it; the files
are UTF-8. `DiffDefinitions` ignores the declaration, because the COM API
hands it a decoded string.

| Case | What it holds |
| :--- | :--- |
| `consumer` | magic-cli-remote's definition shape: logon and repeating time triggers, its settings, one `Exec` |
| `triggers` | every trigger kind a user may register for themselves, each element set to a value that is not its default |
| `settings` | every setting set to a value that is not its default |
| `defaults` | every setting written at its documented default |
| `minimal` | only `Actions`, which is all the schema requires |
| `comhandler` | a `ComHandler` action |
