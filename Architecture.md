# Architecture

## Resource deletion

### Criteria for pruning

- `CreateContainerConfigError`
  - A container could not be created due to errors in the resource definition
  - Happens when e.g., you try to reference a config map that doesn't exist/is missing keys
- `ImagePullBackOff`/`ErrImagePull`
  - Happens when a container cannot find/pull an image from its registry, usually terminal
  - This check is for both containers in a deployment and their init containers
- `CrashLoopBackOff`
  - Happens when the application inside the container crashes and/or restarts, see restart threshold below
  - This check is for both containers in a deployment and their init containers

### Runtime checks

- Resource age (default 10 minutes)
  - Any resources younger than this threshold will not be checked
- Last notification sent (default 24 hours)
  - If a resource (currently only deployments) has been annotated with `nais.babylon/last_notified` it is skipped while the notification is younger than the configured value
- Restart threshold (default 200 restarts)
  - During `CrashLoopBackOff` the pod will be ignored while the number of restarts is less than the threshold
- Tick rate (default 15 minutes)
  - The tick rate is the duration for which the application's main loop will wait between each run (somewhat similar to `Time.sleep`)
  