#!/bin/bash
# Where the scenario gate writes its results and logs. Sourced by the gate and
# by the upload check, so the two cannot disagree about which files exist.
: "${RESULTS:=/tmp/scenario_gate_results.tsv}"
: "${LOG_DIR:=/tmp/scenario_gate_logs}"
