# Quobyte CSI and Quobyte Cluster Versions

Quobyte CSI supports both Quobyte 2.x and 3.x.
However, when using Quobyte CSI driver against Quobyte 2.x cluster,
you should disable Quobyte 3.x features (comment lines in yaml).

Current 3.x features:

* labels (StorageClass.parameters.labels)
