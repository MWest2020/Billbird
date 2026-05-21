# billbird-labels-passthrough

Mirror GitHub issue labels to time_entries and plan_entries via a TEXT[] column so reports can slice on any label-prefix (strippenkaart:*, wbso:*, type:*, internal:*). No new tables, no budget tracking in Billbird; labels are GitHub's source of truth and we snapshot them at log/plan time.
