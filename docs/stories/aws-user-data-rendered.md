---
kind: code
status: ready
epic: aws-layer-neutral-hcl
depends_on: []
touches: ["internal/cli/aws_user_data.go (new)", "internal/cli/aws_user_data_test.go (new)", "internal/cli/generate_command.go", "internal/generator/generator.go", "internal/generator/prompt.go", "internal/generator/claude_adapter.go", "internal/generator/openrouter_adapter.go", "internal/generator/aws_user_data_prompt_test.go (new)", "prompts/aws/phase2_generate_hcl.md", "docs/stories/aws-user-data-rendered.md (delete)"]
risk: high
---

# infrafactory renders infrafactory-user-data.sh from service:, and only service scenarios are told to reference it

New internal/cli/aws_user_data.go renders an AL2023 script from scenario.ServiceSpec (internal/scenario/service.go:42-66): dnf install docker, systemctl enable --now docker, then docker run -d --restart=always -p <port>:<port> with image:tag single-quoted. It refuses an image or tag outside a conservative charset. generateAndWriteFilesWithResult (generate_command.go:668) extends scenarioMeta (:682) with service. For cloud aws with a service, it refuses model files that clean to infrafactory-user-data.sh, then adds the rendered bytes to the file map, so the run store snapshots them. For aws without a service, it refuses any generated .tf that references infrafactory-user-data.sh; that becomes repair feedback (run_command.go:1143-1160) instead of a late file() failure. resetGeneratedFilesIncremental (:972-993) removes a regular file at that path and refuses a symlink or directory. The write is O_CREATE|O_EXCL|O_NOFOLLOW, not os.WriteFile (:964). The phase-2 prompt never sees the scenario YAML: it renders ArchitecturePlan, Pitfalls, ProviderSchema and FeedbackJSON (prompts/aws/phase2_generate_hcl.md:1-30). So generator.Request (generator.go:27-35) gains UserDataLine, set by generate only for aws + service. PromptContext (prompt.go:10-24) gains the same field, both renderPhasePrompt builders copy it (claude_adapter.go:194-213, openrouter_adapter.go:158-177), and phase2 wraps the instruction in {{if .UserDataLine}}. One exported constant holds the exact line user_data = file("${path.module}/infrafactory-user-data.sh"). aws-phase1-literals rebases onto these generator edits. ADR: none — implements aws-layer-neutral-decisions.

**Done when:**
- Golden test: nginx / 1.27 / 80 renders the exact script. Image `nginx;curl x|sh`, tag `1.27 && id`, an image with a newline and a tag with a quote each refuse, naming the field
- renderPhasePrompt on BOTH ClaudeSeedGenerator and OpenRouterSeedGenerator, with the real prompts/aws/phase2 template, contains the constant for a Request with UserDataLine set. For aws-instance.yaml's Request (no service) it contains neither the constant nor infrafactory-user-data.sh. An adapter that forgets to copy the field fails
- A fake-generator generate for an aws scenario with service: sets Request.UserDataLine and writes the golden script, in clean and in incremental mode. A stale script from the previous iteration is replaced
- A fake-generator generate for aws-instance.yaml (no service) leaves Request.UserDataLine empty. Model HCL that references infrafactory-user-data.sh fails generate with an error naming the missing service: block
- Model output naming infrafactory-user-data.sh, ./infrafactory-user-data.sh or sub/../infrafactory-user-data.sh fails generate naming the path, and nothing is written
- In incremental mode a symlink at the path fails generate, and the link target's bytes are unchanged
- A Scaleway scenario with service: writes no script and gets no UserDataLine. Script bytes are identical with sandbox_deploy.enabled true and false
- TestPromptTemplateFieldsExistOnPromptContext and TestPromptTemplatesRenderAgainstZeroValueContext pass
