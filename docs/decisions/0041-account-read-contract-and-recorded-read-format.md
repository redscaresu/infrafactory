# ADR-0041: The Account-Read Contract, the Read-Set and Recorded-Read Formats

## Status
Accepted

## Context

`infrafactory rehearse read` reads a named AWS account through one walled seam and writes what it
read under `.infrafactory/`. Three consumers share the format: fakeaws's request log (which records
the same shape for requests it receives), fakeaws's `POST /mock/load` (which loads a recorded read),
and discovery (which names what to read). A format each of them defined for itself would drift, so
it is defined once, here. Implementations parse the fenced JSON examples below strictly.

## Decision

### Read-set entry

A read-set entry is `{service, protocol, action, params}`, with `protocol` one of `query`, `json` or
`rest`.

- **query:** `action` is the form body's `Action`. `params` is a JSON object of every other form
  field except `Version`, keys exactly as on the wire (`Filter.1.Name`, `GroupName.1`, `Owner.1`),
  string values. A repeated key is refused.
- **json:** `action` is the operation after the dot in `X-Amz-Target`. `params` is the request
  body's JSON object, verbatim.
- **rest:** `action` is `"<METHOD> <route pattern>"`, and `params` is `{path, query}`: the raw path
  and the raw query string. So two S3 subresources on one bucket are two entries that differ only in
  `params.query`.

Rest entries are outside the reader table, so the reader refuses every rest entry by name.

```json
{
  "service": "ec2",
  "protocol": "query",
  "action": "DescribeVpcs",
  "params": {"Filter.1.Name": "tag-key", "Filter.1.Value.1": "env"}
}
```

```json
{
  "service": "ssm",
  "protocol": "json",
  "action": "GetParameter",
  "params": {"Name": "/app/db/host", "WithDecryption": true}
}
```

```json
[
  {
    "service": "s3",
    "protocol": "rest",
    "action": "GET /s3/{bucket}/",
    "params": {"path": "/s3/web-assets/", "query": "tagging"}
  },
  {
    "service": "s3",
    "protocol": "rest",
    "action": "GET /s3/{bucket}/",
    "params": {"path": "/s3/web-assets/", "query": "policy"}
  }
]
```

### Recorded read

A recorded read is `{account, region, timestamp, duration_ms, calls, exchanges}`. Each exchange is a
read-set entry plus `status` (the wire status) and `body`, where `body` is the raw response bytes as
a JSON string. `calls` counts every request sent, retries, identity and preflight included. The
`account` and `region` are what fakeaws `/mock/load` stores as the loaded identity.

```json
{
  "account": "123456789012",
  "region": "eu-west-1",
  "timestamp": "2026-10-11T09:30:00Z",
  "duration_ms": 842,
  "calls": 3,
  "exchanges": [
    {
      "service": "ec2",
      "protocol": "query",
      "action": "DescribeVpcs",
      "params": {"Filter.1.Name": "tag-key", "Filter.1.Value.1": "env"},
      "status": 200,
      "body": "<DescribeVpcsResponse><vpcSet></vpcSet></DescribeVpcsResponse>"
    }
  ]
}
```

### Where params live

Params and bodies never reach fakeaws `/mock/state`, whose audit stays
`{service, protocol, action, status, class}`, nor its db or snapshot. They stay in process memory,
in the token-gated request log only.

### Files

Everything is written under `.infrafactory/`, directories 0700 and files 0600.

### `rehearse read` flags

```
infrafactory rehearse read --account <id> [--principal <arn>] --region <r> \
  --credentials <file> --read-set <file> --out <dir>
```

`--out` must resolve under `.infrafactory/`. The account and region come from flags, never from
`infrafactory.yaml`'s `aws:` block. This ADR records the flags, so the CLI PR carries `ADR: none`.

## Consequences

- One format serves the reader, fakeaws's request log, `/mock/load` and discovery; a change to it is
  an amendment to this ADR.
- Rest entries cannot be read until the reader table grows a rest row, which is a new decision.
- Keeping params out of `/mock/state`, the db and the snapshot means a loaded or snapshotted mock
  never carries values read from a real account.
