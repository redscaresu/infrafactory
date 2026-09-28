# Review — AWS credential preflight: sts:GetCallerIdentity refuses a wrong account or principal

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

## Declined: [P1] "Use a non-proxying STS transport"

The claim: the default STS client (`&http.Client{Timeout: awsSTSTimeout}` in
`buildRuntime`) uses Go's default transport, which honours `HTTP_PROXY` and
`HTTPS_PROXY`, so an ambient proxy could redirect the credential proof.

The proxy variables pass through by design. The aws-layer3-seal-and-dispatch epic
keeps `HTTP_PROXY`, `HTTPS_PROXY` and `SSL_CERT_FILE` in the sealed environment
because they are not `AWS_*`: tofu and terraform-provider-aws reach AWS through
them, on an operator network that requires a proxy for egress. A preflight that
ignored them would check a different network path from the one the apply uses,
and would refuse exactly the hosts where the apply works.

A proxy cannot retarget or forge the answer. The request goes to
`https://sts.<region>.amazonaws.com`, a hostname built from a validated region and
never read from config or env. Through a proxy, HTTPS is a CONNECT tunnel, and TLS
is verified end to end against that hostname. The proxy sees an opaque stream.
Forging the answer would take a CA the process trusts, and a trusted CA subverts
tofu's own traffic equally. The seal exists to stop endpoint redirection:
`AWS_ENDPOINT_URL_*`, the profile `services` section and IMDS. Those three can
point a correctly signed request at a server that answers anything. A proxy cannot.
