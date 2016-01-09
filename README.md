# Telegraf plugin: Announced

#### Plugin arguments:
```
interface = "eth0"
destination = "ff02:0:0:0:0:0:2:1001"
port = 1001
timeout = 3
```

#### Description

Announced is a Gluon (OpenWRT) service, get fetch information of router.
(often used by Freifunk Communities)

# Measurements:

Meta:
- tags: `node=<mac>`

- announced_statistics
- announced_nodeinfo
- announced_neighbours
