---
title: On-call handover
tags: [work, on-call]
---

Before handing over the pager on Monday morning:

1. Close or re-home every incident still open in the tracker.
2. Check the backup job ran on every database.
3. Write down anything that paged twice, even if it fixed itself.

Weekend pages count too, even the ones that cleared on their own: the
next person will meet them again on a Tuesday afternoon.

## Checking the database backups

```sh
for db in orders billing auth; do
  aws s3 ls "s3://backups-prod/$db/" | tail -n 1
done
```

The newest object for each should be from this morning. If one is missing,
the restore runbook is in the team wiki; page the database owner before trying
it alone.
