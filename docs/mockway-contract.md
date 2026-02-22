# Mockway Integration Contract (InfraFactory Side)

This document captures only what InfraFactory needs from Mockway on a fresh context.
Full Mockway implementation details live in the Mockway repository.

## Purpose

InfraFactory uses Mockway as a deterministic Scaleway API target for harness validation in mock deploy layers.

## Required behavior

1. Base API routing
- Mockway must accept Scaleway provider calls via a single base URL (set through `SCW_API_URL`).

2. Auth behavior
- Scaleway API routes require `X-Auth-Token` with any non-empty value.
- Admin routes under `/mock/*` are unauthenticated.

3. Admin endpoints used by InfraFactory
- `POST /mock/reset`: reset mock state before iterations/tests.
- `GET /mock/state`: retrieve full state for topology/policy/destruction checks.
- `GET /mock/state/{service}`: optional service-scoped inspection.

4. State expectations
- Resource graph relationships are preserved in `/mock/state` output.
- Reset clears previously created resources.
- Create/delete behavior should be stateful and deterministic.

## InfraFactory assumptions

- Harness sets fake Scaleway credentials and `SCW_API_URL` before `tofu` commands.
- Generated OpenTofu should remain provider-normal (no Mockway-specific provider logic in generated code).
- Topology and policy evaluation run in InfraFactory, not in Mockway.

## Ownership boundary

- InfraFactory owns orchestration and evaluation logic.
- Mockway owns API compatibility and backing state behavior.

## Source references

- Mockway repo: `github.com/redscaresu/mockway`
- InfraFactory design narrative: `CONCEPT.md`
