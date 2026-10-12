---
kind: code
repo: fakeaws
status: blocked
blocked_by: [fakeaws-load-images]
epic: fakeaws-loader
depends_on: [fakeaws-load-images]
touches: [handlers/ec2.go, handlers/ec2_test.go, handlers/recorder.go, handlers/recorder_test.go, handlers/load_ec2_compute.go, handlers/load_ec2_compute_test.go, repository/ec2_volumes.go, repository/ec2_volumes_test.go, repository/ec2_tags.go, repository/load_ec2.go, repository/testdata/schema.golden, coverage_matrix.yaml, testdata/ec2/DescribeVolumes.xml]
---

# D7 for a loaded instance: its root volume loads from DescribeVolumes, DescribeVolumes answers it and terminate removes it

The recorded source is prod-read-seam-reader-describe-volumes's DescribeVolumes follow-up.

**Table.** New ec2_volumes: id, AZ, size, type, iops, throughput, encrypted, KMS key, snapshot,
state, createTime, attachments, multiAttach, and tags through ec2TaggedTables and a trigger; in
resetTables. It is a new table, so there is no schema bump; it is appended to the golden.

**DescribeVolumes** leaves ec2NoneSets and the none-set dispatch (handlers/ec2.go:164-165,
:3358-3392) for a real handler: `VolumeId.N`, where an unknown id is InvalidVolume.NotFound, and
the filters volume-id, attachment.instance-id, attachment.delete-on-termination, status,
availability-zone and tags.

**Loader** for DescribeVolumes, so an instance's blockDeviceMapping now resolves; an attachment
naming an absent instance is refused. A volume's snapshotId and kmsKeyId are references the reader
never reads, answered under unresolved: snapshots are out of scope (the epic), and KMS keys live
in kmsStore, not rows (repository/identity.go:31-33). Real root volumes nearly always carry a
`snap-`, and often a customer-managed key.

**Terminate.** TerminateInstances deletes the instance's DeleteOnTermination volumes in its
transaction and detaches the others (rows stay on terminate today, :2913-2970). There is no
CreateVolume: read and terminate only.

**Classes.** On a loaded db DescribeVolumes is answered-from-rows; on an unloaded db it stays
empty-collection, since RunInstances still makes no root volume. That changes the docstring at
handlers/recorder.go:47-51, TestContract_request_log_empty_collection,
TestRegressionSweepDescribesNeverCreated and coverage_matrix's aws_ebs_volume row. /mock/state
gains `ec2.volumes`, so orphan counts see a leftover volume (infrafactory
internal/generator/orphan_classify.go:273-305 counts any non-empty collection).

**Done when:**
- An instance loaded with its DescribeVolumes pair loads; DescribeVolumes by `VolumeId.1` and by
  `attachment.instance-id` answers the recorded body by load_diff.go in both directions.
- A volume recorded with `snap-0abc` and a customer-managed KMS key ARN loads, and the answer names
  both under unresolved.
- TerminateInstances removes the DeleteOnTermination root volume, after which `VolumeId.1` answers
  InvalidVolume.NotFound.
- Mutate once: a volume with DeleteOnTermination false stays after terminate, and DescribeVolumes
  equals the recording except at status (available) and the attachment.
- Under RecordRequests, DescribeVolumes is answered-from-rows on a loaded db and empty-collection on
  an unloaded one; TestContract_request_log_empty_collection asserts both.
- /mock/state lists the loaded volume under `ec2.volumes`, and not after terminate.
- TestFullCoverageAudit passes, and the aws_ebs_volume row no longer says fakeaws creates no
  volumes.
