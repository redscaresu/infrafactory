terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.83.0"
    }
  }
}

provider "scaleway" {
  region = var.region
  zone   = var.zone
}
# NOTE: the provider pin above was updated by hand from 2.81.0 to 2.83.0
# when the pin moved (S188). Everything else here is as the model
# produced it. A hand-edited line in a recording is worth flagging:
# re-record with `make demo-gate GENERATE=live` when the LLM next runs
# for this scenario, so the file goes back to being purely a recording.
