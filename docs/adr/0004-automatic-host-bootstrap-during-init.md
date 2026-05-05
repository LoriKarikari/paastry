# Automatic host bootstrap during init

PaaStry runs Docker host bootstrap automatically during `paastry init`: it initializes Swarm when needed, creates the default tenant overlay network, narrates each host mutation, and fails init if Docker cannot be prepared. This chooses self-hosted PaaS convenience over a stricter operator-prepared-host model while keeping host mutation confined to init rather than `paastry server`.
