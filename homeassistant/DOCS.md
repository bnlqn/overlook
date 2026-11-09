# Overlook

Overlook is one page that tells you what is wrong in your homelab, read
from the tools it already runs. This app runs it inside Home Assistant and
opens it in the sidebar. Everything else about it, every tool it reads and
every key of its config, is in
[its README](https://github.com/bnlqn/overlook#readme).

## Installing

1. In Home Assistant, open **Settings**, **Apps**, **App store**, then the
   menu at the top right, **Repositories**, and add
   `https://github.com/bnlqn/overlook`. Or open
   [this link](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fbnlqn%2Foverlook),
   which opens that dialog in your own Home Assistant with the address
   filled in.
2. Find Overlook in the store, install it, turn on **Show in sidebar**, and
   start it.
3. Open it from the sidebar. With no config yet, it shows its setup page.

## Setting it up

On the setup page, pick the tools your homelab runs, give each its address
and key, and try it. From the sources that answered, the setup page puts
together two files, `overlook.yaml` and the `.env` that holds their keys.
Overlook writes nothing, so you save them:

- **Where:** in Overlook's own folder, which Home Assistant keeps in
  `app_configs`, named after this repository and the app, such as
  `0a1b2c3d_overlook`. Overlook reads it, read-only, as `/etc/overlook`.
- **How:** with Home Assistant's Samba share app, in its `app_configs`
  share, or with its Terminal & SSH app, as
  `cd /app_configs/*_overlook`, then `nano overlook.yaml` and `nano .env`.
  The files must be readable by others, as they are by default, since
  Overlook runs as a user of its own.

Then restart Overlook on its page in **Settings**, **Apps**. The setup page,
still open, turns into the page. If it doesn't, the app's **Log** tab says
why: a config that is there but wrong stops Overlook, and says what is wrong
with it.

From then on, you change the config in its file, and restart the app. Your
own icons and `custom.css` go in the same folder
([Your own look](https://github.com/bnlqn/overlook#your-own-look)), and so
does a CA a source needs, given as `ca: /etc/overlook/<file>`.

## Addresses

Overlook runs in a container on Home Assistant's own network, so
`localhost` is Overlook itself. Give each source the address of the machine
it runs on, or a name your network knows it by. Home Assistant is
`http://homeassistant:8123` from there, for Overlook to read it as a source
([Home Assistant](https://github.com/bnlqn/overlook#home-assistant)).

## Outside the sidebar

In the sidebar, Overlook is behind Home Assistant's login, and only Home
Assistant's administrators see it. To open the page in a browser outside
Home Assistant, add it to a phone's home screen, or let an agent read it
through MCP ([Agents, through MCP](https://github.com/bnlqn/overlook#agents-through-mcp)),
give the app's port a number in its **Network** settings, such as `8080`.
Overlook answers that port with no login, to anyone who can reach it
([No login](https://github.com/bnlqn/overlook#no-login)), so open it only
where your network is your own. MCP is then at `/mcp` on that port, such
as `http://homeassistant.local:8080/mcp`.

## Updates and backups

Each Overlook release comes to the app as an update. Its notes are with
[the releases](https://github.com/bnlqn/overlook/releases), and one that
breaks a config says so first.

Home Assistant's backups of the app hold its folder, so `.env` and its keys
too: keep them encrypted, as Home Assistant does by default. Uninstalling
the app with its data deletes the folder.

## Help

Questions go to
[Overlook's discussions](https://github.com/bnlqn/overlook/discussions),
and bugs to [its issues](https://github.com/bnlqn/overlook/issues). A
vulnerability goes through
[its security policy](https://github.com/bnlqn/overlook/security/policy),
never in public.

Overlook is under the MIT licence.
