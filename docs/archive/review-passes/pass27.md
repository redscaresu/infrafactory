# S165 review pass 27 — one finding, acted on

### [P2] The project was created before the environment was validated

`ensureRunProject` ran before `sandboxCommandEnvForProject` had checked the
sealed environment. With `SCW_SECRET_KEY` and `SCW_DEFAULT_ORGANIZATION_ID` set
but `SCW_ACCESS_KEY` missing, the command would create a **real project**, then
fail preflight, and rely on best-effort cleanup for residue that should never
have existed — and a delete failure or an interruption would leave it behind.

The environment is now validated first, and the project is created only once it
passes; the env is then rebuilt with the project id. Building it twice is cheap
next to an Account API side effect from a configuration that was always going to
be rejected.

A good example of the class of bug this loop keeps finding: not a wrong
computation, but an operation ordered so that a failure leaves something behind.
