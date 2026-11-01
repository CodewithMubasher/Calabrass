# Using Calabrass from another machine

By default nothing but the machine Calabrass runs on can reach it. This page is for when you want your laptop's terminal (or a script on a server) to use the agent on your main machine.

## How it works

A machine that is not the host gets in only by being **paired**:

1. Start the host with remote access allowed and a listen address other machines can reach:
   ```bash
   CALABRASS_ALLOW_REMOTE=1 CALABRASS_ADDR=0.0.0.0:8787 calabrass-backend --daemon
   ```
   (Without `CALABRASS_ALLOW_REMOTE=1` the host refuses to listen beyond this machine at all.)
2. On the host machine: `cal pair`. It prints a code like `K7QD-M2XH` and the address to use. The code works **once** and for **ten minutes**.
3. On the other machine: `cal connect 192.168.1.20:8787 K7QD-M2XH`. The machine is now a **device**: `cal` and `cal run` there use that host, and never start one of their own.
4. On the host machine: `cal devices` shows who is paired, and `cal revoke <id or name>` removes one. It is signed out immediately.

`cal disconnect` on the device goes back to its own host. `cal --local` uses the local host once without disconnecting.

## What a paired device can and cannot do

It is signed in as **you**: it sees your chats, uses your providers, and the agent works in folders on the **host** machine, not on the device. It is as trusted as your own terminal there, so pair only machines you own.

It can **not**:

- make pairing codes, list or remove devices, or stop the host (those only work on the host machine itself);
- create accounts or sign in with a password (those are refused from any other machine);
- do anything after you revoke it, or after 90 days without re-pairing.

Actions that need your approval still do: an approval the agent raises can be answered from any paired device or from the host's own windows.

## What protects the pairing

- Codes are 8 symbols from a 28-symbol alphabet with the confusable ones left out, so about 38 bits, and are only ever stored as a hash.
- A code is single-use and expires in ten minutes. Using one up is atomic, so two machines presenting the same code cannot both win.
- Claiming a code is rate-limited: 5 tries a minute per address, 40 a minute overall. Every wrong code gets the same answer, so guessing teaches nothing.
- Each device has its own session. Removing the device deletes its sessions, so it stops working at once.
- Pairing and removals are recorded in the [ledger](PERMISSIONS.md#the-ledger).

## The one thing to be careful about: it is not encrypted

The connection is plain HTTP. The device's token, your conversations, and whatever the agent reads from your files cross the network readable. So:

| Where | Is it OK? |
|---|---|
| Same machine, or through an SSH tunnel | Yes. |
| A VPN such as Tailscale or WireGuard | Yes. The VPN encrypts it. `cal connect` treats `100.64.0.0/10` and `*.ts.net` as safe. |
| Your home network | Fine, with care: anyone on it can read the traffic. `cal connect` warns. |
| The open internet | **No.** `cal connect` refuses a public address unless you add `--insecure`. |

The simplest safe setup is an SSH tunnel, and it needs no remote-access setting beyond pairing:

```bash
# on the device
ssh -N -L 8787:127.0.0.1:8787 you@host-machine &
cal connect 127.0.0.1:8787 K7QD-M2XH    # code from `cal pair` on the host
```

For that, the host can stay on `127.0.0.1` (still start it with `CALABRASS_ALLOW_REMOTE=1` so it will issue codes).

Built-in encryption and a relay that needs no open port are planned; until then, use one of the options above.

## If something goes wrong

- *"can't reach a Calabrass host"*: it is not running, the address is wrong, or a firewall is in the way.
- *"does not accept other devices"*: the host was not started with `CALABRASS_ALLOW_REMOTE=1`.
- *"not valid, or it has expired"*: codes are single-use and short-lived; make a new one.
- *"no longer accepts this device"*: it was removed (or 90 days passed). Pair again.
- *Disconnected while using it*: the terminal shows "Host disconnected · reconnecting" and tries again by itself. For a host on another machine it never tries to start one; it waits for it.
