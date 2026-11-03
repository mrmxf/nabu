---
title: The agent task
params:
  readiness: production
  kind: agent
  agent_key: echo
  sim_output: |
    **echo returned 2 facts**

    | Fact | Value |
    |---|---|
    | One | 1 |
    | Two | 2 |
---
Run the agent and compare its output with your own.
