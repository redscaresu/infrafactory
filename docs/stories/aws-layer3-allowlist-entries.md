---
kind: code
status: ready
epic: aws-layer3-gate
depends_on: [layer3-gate-shared-rules, aws-web-live-scenario]
touches: ["internal/config/config.go", "infrafactory.yaml", "docs/layer3-coverage.md", "internal/cli/layer3_coverage_doc_test.go", "internal/cli/layer3_allowlist_test.go", "internal/e2e/aws_allowlist_schema_test.go (new)", ".github/workflows/ci.yml", "docs/stories/aws-layer3-allowlist-entries.md (delete)"]
risk: high
---

# The nine AWS types join allow_resource_types, aws_eip included and placed by its arn; the coverage doc and its tests admit aws_; each entry is proven placeable against the real hashicorp/aws 5.100.0 schema

- config.go:314-328 and infrafactory.yaml:87-177 gain exactly aws_vpc, aws_subnet, aws_internet_gateway, aws_route_table, aws_route, aws_route_table_association, aws_security_group, aws_instance and aws_eip. They are exact names with no glob, and one comment cites the HLD gate section.
- aws_eip is decided: admitted and placed by its arn. In 5.100.0 its arn is built client-side by RegionalARN from the provider's own account (ec2_eip.go:245,445-447 at tag v5.100.0), and UnplacedAWSResources places by arn (aws_placement.go:80-104). For aws_eip that check is only as strong as the credential, and an EIP cannot be allocated in any other account.
- In 5.100.0 the other six non-child types carry arn and owner_id (aws_instance carries arn). aws_route and aws_route_table_association carry neither, and are on AWSChildScopedTypes (aws_placement.go:14-17).
- docs/layer3-coverage.md: the title (:1) covers both clouds, and the Repo default prose (:52-57) lists the aws_ entries. aws-web-live gets a 'runnable, unrun' row, and the totals line (:129) is updated.
- layer3_coverage_doc_test.go:36: allowlistEntryRe matches (scaleway|aws)_, and TestAllowlistEntryRegexAcceptsDigits gains aws_route_table_association.
- layer3_allowlist_test.go: every entry starts with scaleway_ or aws_, and no aws_ entry contains *.
- New internal/e2e/aws_allowlist_schema_test.go:
  - an ungated test of the pure unplaceableAWSTypes(schema, allowlist, children);
  - TestE2E_AWSAllowlistPlaceable, which reads the real 5.100.0 schema through the SealNetwork mirror (helpers.go:506-533) and joins the CI 'AWS against fakeaws' list (ci.yml:105).

Dependency notes: aws-web-live-scenario (aws-ingress-policy-and-holdout)

**Done when:**
- TestLayer3CoverageDocTotalsMatchItsTable, TestLayer3CoverageDocAllowlistMatchesConfig and TestLayer3DefaultAllowlistMatchesCheckedInConfig pass with the aws_ entries. Removing one aws_ entry from the doc or from infrafactory.yaml fails them
- The new prefix test in layer3_allowlist_test.go fails on a gcp_ entry and on an unprefixed entry, and the new glob test fails on aws_*
- unplaceableAWSTypes names a synthetic type that carries neither arn nor owner_id and is not a child. It passes a type with arn, a type with owner_id, and aws_route
- TestE2E_AWSAllowlistPlaceable passes in the CI 'AWS against fakeaws' step against the real 5.100.0 schema, and aws_eip passes by its arn attribute
- layer3_hcl_shape_test.go and TestLayer3AllowlistDoesNotContradictStaticPolicy pass unmodified with the widened default list
- doc-hygiene passes, and the tip commit carries 'ADR: none — ADR-0023 rule 5 list widened per HLD 2026-09-27 § The gate; ADR-0023's AWS record is aws-layer3-claim-sweep-reap's'
