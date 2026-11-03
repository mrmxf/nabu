# Image server setup — the one-off for pixie's "make an image"

This is the ollama-equivalent runbook for the **image** side: a one-time setup of
the txt2img server that pixie's image mode calls. It mirrors how Ollama is set up
(a loopback model server on the worker box that ni6 reaches over HTTP), and it's
written for the worker host — the home **GPU-workstation running ni6 in Debian
WSL** (`gpu-worker`, RTX-4080 16 GB).

**What already exists on this box (verified):** ni6 is deployed and running under
systemd, Ollama is up on `:11434` with the model resident. The only missing piece
is the txt2img server on `:7860`. So this runbook is just that piece, plus wiring
ni6 to it and the redeploy checklist.

> **Contract:** ni6 talks to any server that answers the AUTOMATIC1111 API —
> `POST /sdapi/v1/txt2img` with `{prompt, width, height, steps, sampler_name}` →
> `{images: ["<base64 png>"]}`. A1111, **SD.Next**, and **Forge** all speak it.
> ni6 sets `width`/`height` from the group's orientation and always takes the PNG
> back; the size and format are enforced structurally, not by the prompt.

---

## 0. The VRAM reality — read this first

This is the one decision that determines whether the feature is reliable, and it's
the same "one card, one budget" logic as [agent servers](../help/using-nabu/reference/12-agent-servers.md):

- The RTX-4080 has **16 GB**. Ollama already holds the LLM resident (~9 GB with
  `keep_alive`), leaving **~7 GB** for everything else — including the KV cache for
  in-flight text jobs.
- A full **SDXL** checkpoint (native ~1 MP, so 1280×720 fits well) is ~6.5 GB at
  fp16 and, without offloading, will **not** co-reside with a resident 9 GB LLM.
  Under WSL2, CUDA spills to system RAM instead of hard-OOMing, so contention shows
  up as a **severe slowdown**, not a crash — easy to miss.

So there's no "per-image model-load tax" *as long as both models stay resident* —
the tax appears only when they can't fit together and evict each other on every
text↔image switch. Pick one of:

| Option | VRAM | Trade-off | When |
|---|---|---|---|
| **SD 1.5 + `--medvram`** (recommended to start) | ~2–4 GB | native 512px, so 1280×720 can show duplication; mitigate with hires-fix or a 1.5 model tuned for wide frames | co-resides comfortably with Ollama; prove the pipeline first |
| **SDXL + `--medvram-sdxl`** (Forge/A1111) | ~4–6 GB peak (offloads between stages) | best quality at 1280×720; tight alongside 9 GB Ollama — cut `OLLAMA_NUM_PARALLEL`/KV if you see spillover | quality matters and you can spare the headroom |
| **Separate GPU / host** | — | eliminates contention entirely; `NABU_AGENT_IMAGE_HOST` can point anywhere | if you outgrow one card |

**Recommendation:** stand it up with **SD 1.5 first** to prove end-to-end, then
evaluate SDXL once you can watch `nvidia-smi` under real load. **Forge**
(`lllyasviel/stable-diffusion-webui-forge`) is the most VRAM-frugal A1111-compatible
option and is the better long-term choice on a shared 16 GB card.

---

## 1. Install (Debian WSL, unprivileged)

Run as a normal user (like ni6, this must **not** be root — it's a large Python
dependency tree). AUTOMATIC1111 shown; Forge/SD.Next are the same shape.

```bash
# prerequisites (Debian bookworm): python venv + git are already present on this box;
# the WebUI's own launcher creates its venv and pulls torch+cuda wheels.
sudo apt-get update && sudo apt-get install -y python3-venv libgl1 libglib2.0-0

# clone somewhere on the worker box (NOT under /opt/ni6 — that's ni6's sandbox)
git clone https://github.com/AUTOMATIC1111/stable-diffusion-webui ~/sd-webui
cd ~/sd-webui

# grab an SD 1.5 checkpoint to prove the pipeline — the emaonly (inference-only) build
# is ~4 GB and co-resides comfortably with Ollama (see §0). ~1.9 GB download.
wget -P models/Stable-diffusion/ \
  https://huggingface.co/stable-diffusion-v1-5/stable-diffusion-v1-5/resolve/main/v1-5-pruned-emaonly.safetensors

# Other checkpoints work the same way — drop any SD 1.5 / SDXL / Forge-compatible
# .safetensors into models/Stable-diffusion/ (download per that model's licence).
# SDXL gives the best 1280×720 quality but is tighter on this shared card (§0).
ls models/Stable-diffusion/*.safetensors   # expect at least the file above
```

## 2. Run it — API on, loopback only

ni6 runs on the **same box**, so the image server only needs loopback (like Ollama).
Do **not** expose it to the tailnet.

```bash
# --api enables /sdapi/*; --nowebui drops the browser UI (lighter); --medvram for 16 GB
# shared with Ollama. Port 7860 matches NABU_AGENT_IMAGE_HOST's default.
./webui.sh --api --nowebui --port 7860 --medvram
#   SDXL: use --medvram-sdxl instead of --medvram
#   Forge: same flags; its launcher is ./webui.sh too
```

First launch builds the venv and downloads torch — several minutes, once. Leave it
running (a systemd unit for durability is in step 5).

## 3. Prove it

```bash
# reachable + a checkpoint is loaded (this is exactly what ni6/start.sh probes)
curl -fsS http://127.0.0.1:7860/sdapi/v1/sd-models | grep -o '"title"' | wc -l   # >= 1

# a real 1280x720 render round-trips to base64 PNG
curl -fsS -X POST http://127.0.0.1:7860/sdapi/v1/txt2img \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"a school newsroom desk, photographic","width":1280,"height":720,"steps":24}' \
  | grep -o '"images"' && echo TXT2IMG_OK
```

Watch `nvidia-smi` during that render: if used memory pins at 16 GB and the render
crawls, you're in VRAM spillover — drop to SD 1.5, add `--medvram-sdxl`, or lower
Ollama's `OLLAMA_NUM_PARALLEL`.

## 4. Wire ni6 to it

The default already points at this server (`NABU_AGENT_IMAGE_HOST=http://127.0.0.1:7860`,
baked into `ni6/deploy/ni6.service` (not yet in this repo)), so:

```bash
sudo systemctl restart ni6
/opt/ni6/start.sh --check      # the pre-flight now shows: image server  reachable at … (N checkpoint(s))
journalctl -u ni6 --since "20 seconds ago" --no-pager
```

`start.sh` treats a missing image server as a **warning, not a failure** — ni6 and
the text agents run regardless; pixie just serves placeholder PNGs until the server
is up.

## 5. Make it durable (optional systemd unit)

So it survives reboot like Ollama. Run as your normal user (not root); WSL boots
systemd per the existing [agent deploy](../help/using-nabu/reference/11-agent-deploy.md) setup.

```ini
# ~/.config/systemd/user/sd-webui.service  (user unit; enable with: systemctl --user enable --now sd-webui)
[Unit]
Description=txt2img server for pixie images (A1111 API)
After=network-online.target

[Service]
Type=simple
WorkingDirectory=%h/sd-webui
ExecStart=%h/sd-webui/webui.sh --api --nowebui --port 7860 --medvram
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload && systemctl --user enable --now sd-webui
loginctl enable-linger "$USER"      # so the user service runs without an active login
```

(If you'd rather it be a system service ordered before ni6, install it under
`/etc/systemd/system/` as an unprivileged user and add it to `After=` in
`ni6.service` (not yet in this repo) — noted there.)

---

## 6. Don't forget: the rest of the redeploy

The image server only lights up ni6's image path. The **counter, the widget UI, and
the new slide** ship with nabu, not ni6:

1. **ni6** — deploy the new build (adds the image agent):
   `cd ni6 && npm install && npm run build`, then `sudo cp -r dist … /opt/ni6/` and
   `sudo systemctl restart ni6` (or your usual deploy). `start.sh` flags drift.
2. **nabu** — rebuild **and reseed** (the migration auto-applies on serve, but the new
   `72-pixie-image` slide and `images_budget: 15` only land via seed):
   `./nabu/scripts/build.sh` then `go run ./nabu seed` (or rebuild the deploy snapshot
   from current content — a redeploy carrying an old `pb_data` won't have them).
3. **image server** — this runbook.

## 6a. Turning it off — unload SD and hide the feature

Testing showed SD and Ollama **resident together thrash a single 16 GB card** (the
§0 spillover, but under real classroom load), so image generation ships **off by
default**. Turning it off is two independent steps — free the VRAM, and hide the
UI:

**Unload the model (reclaim VRAM).** The webui process holds the VRAM for as long
as it runs, so stopping it is the unload:

```bash
# if you installed the user unit from step 5:
systemctl --user disable --now sd-webui      # stop now + don't restart on boot

# if you ran it by hand instead:
pkill -f webui.sh                            # or the python child if it lingers

nvidia-smi                                    # confirm: only Ollama's ~9 GB remains
```

ni6 treats a missing image server as a **warning, not a failure** (`start.sh`), so
text agents keep running; pixie would just fall back to placeholder PNGs.

**Hide the feature (nabu).** So students never see a dead "make an image" option,
gate it in `nabu-config.yaml`:

```yaml
ai:
  ollama:
    enabled: true
  stable-diffusion:
    enabled: false      # hides the pixie-image slide, the image-budget teacher
                        # card, and the "make an image" widget; the job API also
                        # refuses image submits while this is false
```

This seeds the `images_enabled` app_config row, so it takes effect on **reseed**
(`go run ./nabu seed`, or a redeploy carrying a fresh snapshot) — not just a
restart. Flip it back to `true` and reseed once the image server has its own VRAM
headroom (a second GPU/host, or Ollama sized down).

## 7. Classroom notes (same discipline as Ollama)

- **Pre-warm**: fire one throwaway render at lesson start so student one doesn't eat
  the checkpoint load.
- **Budget**: each group gets `images_budget` images (15 by default, authored in the
  lesson `_index.md`); teachers adjust a group's remaining count live from the
  dashboard Queues tab. That metering is what keeps a shared GPU from being swamped.
- **Latency knob**: `NABU_AGENT_IMAGE_STEPS` (default 24) scales roughly linearly; drop it
  for speed, raise it for quality.
