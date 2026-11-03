---
title: The human task
params:
  readiness: production
  kind: human
  timers:
    - label: Task
      seconds: 900
  t_choice:
    - id: human-topic
      legend: Pick your topic
      scope:  group
      default_value_key: slug
      options:
        - { key: A, label: "Topic A", slug: "topic-a" }
        - { key: B, label: "Topic B", slug: "topic-b" }
        - { key: C, label: "Topic C", slug: "topic-c" }
        - { key: D, label: "Topic D", slug: "topic-d" }
---
Pick the topic for your group:

{{< LMS t = "choice" id = "human-topic" >}}
