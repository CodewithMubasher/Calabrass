import { useEffect, useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  Delete02Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { createAgent, deleteAgent, saveAgent } from "@/lib/agent-store"
import { api } from "@/lib/api"
import { notifyError } from "@/lib/toast"
import type {
  AgentInput,
  AgentRow,
  ProviderInfo,
  SkillInfo,
} from "@/types/chat"

// The dialogs sit on the same dark panel as the reference project's, with our
// own popover token standing in for its neutral-900. This one is wide because
// the two pickers sit side by side.
const SURFACE = "sm:max-w-2xl bg-popover border border-white/10"

type Option = { value: string; label: string }

/**
 * A model is chosen by provider and id, so the two are packed into one value
 * for the select. "::" cannot occur in either, which keeps the split exact.
 */
function encodeModel(provider: string, model: string) {
  return provider && model ? `${provider}::${model}` : ""
}

function decodeModel(value: string): { provider: string; model: string } {
  const at = value.indexOf("::")
  if (at < 0) return { provider: "", model: "" }
  return { provider: value.slice(0, at), model: value.slice(at + 2) }
}

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-white/60">{label}</span>
      {children}
    </label>
  )
}

/**
 * Keeps a saved model in the list even when its provider is no longer
 * connected, so editing an agent never silently drops its choice.
 */
function withSaved(
  options: Option[],
  provider: string,
  model: string
): Option[] {
  const saved = encodeModel(provider, model)
  if (!saved || options.some((option) => option.value === saved)) return options
  return [{ value: saved, label: `${model} (saved)` }, ...options]
}

/**
 * One of the dialog's four pickers. It wraps the shared Select so every field
 * gets the same trigger, the same placeholder, and the same scrollable list.
 */
function Picker({
  label,
  value,
  onChange,
  options,
  placeholder,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: Option[]
  placeholder: string
}) {
  return (
    <Select
      value={value}
      onValueChange={(next) => onChange(next ?? "")}
      items={options}
    >
      <SelectTrigger aria-label={label}>
        <SelectValue placeholder={placeholder} />
        <SelectIcon>
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            strokeWidth={2}
            className="size-3.5"
          />
        </SelectIcon>
      </SelectTrigger>
      <SelectContent>
        {options.length === 0 ? (
          <div className="px-2 py-3 text-center text-xs text-white/30">
            Nothing to choose yet
          </div>
        ) : (
          options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))
        )}
      </SelectContent>
    </Select>
  )
}

/**
 * Creates or edits a sub-agent.
 *
 * Mounted only while it is open, so the new-agent form starts blank and the
 * edit form is filled from the row it was opened with, without an effect.
 */
export function AgentDialog({
  agent,
  onOpenChange,
}: {
  agent?: AgentRow | null
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState(agent?.name ?? "")
  const [prompt, setPrompt] = useState(agent?.prompt ?? "")
  const [description, setDescription] = useState(agent?.description ?? "")
  const [model, setModel] = useState(
    encodeModel(agent?.provider ?? "", agent?.model ?? "")
  )
  const [altModel, setAltModel] = useState(
    encodeModel(agent?.altProvider ?? "", agent?.altModel ?? "")
  )
  const [skills, setSkills] = useState<string[]>(agent?.skills ?? [])
  const [availableSkills, setAvailableSkills] = useState<SkillInfo[]>([])
  const [providers, setProviders] = useState<ProviderInfo[]>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let live = true
    api
      .listSkills()
      .then((list) => {
        if (live) setAvailableSkills(list)
      })
      .catch(() => {
        // The picker simply has nothing to offer; saving still works.
      })
    return () => {
      live = false
    }
  }, [])

  useEffect(() => {
    let live = true
    api
      .listModels()
      .then((list) => {
        if (live) setProviders(list)
      })
      .catch((err) => {
        if (!live) return
        notifyError(
          err instanceof Error ? err.message : "Could not load models.",
          "Could not load models"
        )
      })
    return () => {
      live = false
    }
  }, [])

  // The catalog can be long — every model of every connected provider — which
  // is exactly what the Select's scrollable list is for.
  const modelOptions = useMemo<Option[]>(
    () =>
      withSaved(
        providers.flatMap((provider) =>
          provider.models.map((model) => ({
            value: encodeModel(model.provider, model.id),
            label: `${provider.name} · ${model.label}`,
          }))
        ),
        agent?.provider ?? "",
        agent?.model ?? ""
      ),
    [providers, agent]
  )

  const altOptions = useMemo<Option[]>(
    () => [
      { value: "", label: "No fallback" },
      ...withSaved(
        modelOptions,
        agent?.altProvider ?? "",
        agent?.altModel ?? ""
      ),
    ],
    [modelOptions, agent]
  )

  const ready = name.trim() !== ""

  function toggleSkill(name: string) {
    setSkills((current) =>
      current.includes(name)
        ? current.filter((skill) => skill !== name)
        : [...current, name]
    )
  }

  // Every installed skill, plus any this agent already names that is no longer
  // on the machine: a save must never quietly drop a choice the person made.
  const skillsShown = useMemo(() => {
    const byName = new Map(availableSkills.map((skill) => [skill.name, skill]))
    for (const name of skills) {
      if (!byName.has(name)) {
        byName.set(name, {
          name,
          description: "Not installed on this machine",
          path: "",
        })
      }
    }
    return [...byName.values()]
  }, [availableSkills, skills])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    const preferred = decodeModel(model)
    const fallback = decodeModel(altModel)

    // The mode and the connectors are not asked for here any more: a worker
    // is given them by the crew that runs it. What it already has is sent
    // back untouched, so saving a worker never quietly clears them.
    const input: AgentInput = {
      name: name.trim(),
      prompt: prompt.trim(),
      systemPrompt: prompt.trim(), // Agent prompt IS the system prompt
      mode: agent?.mode ?? "chat",
      description: description.trim(),
      connectorIds: agent?.connectorIds ?? [],
      provider: preferred.provider,
      model: preferred.model,
      altProvider: fallback.provider,
      altModel: fallback.model,
      skills,
    }

    setSaving(true)
    try {
      if (agent) {
        await saveAgent(agent.id, input)
      } else {
        await createAgent(input)
      }
      onOpenChange(false)
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not save the agent.",
        "Could not save the agent"
      )
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    if (!agent || saving) return

    setSaving(true)
    try {
      await deleteAgent(agent.id)
      onOpenChange(false)
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not remove the agent.",
        "Could not remove the agent"
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle className="text-white/90">
              {agent ? "Edit worker" : "New worker"}
            </DialogTitle>
            <DialogDescription className="text-white/50">
              {agent
                ? "Change what this worker is for, and how it works."
                : "Give a worker a role: what it is asked, and which model it thinks with."}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4">
            <Field label="Name">
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Researcher"
                autoFocus
                className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
              />
            </Field>

            <Field label="System prompt">
              <Textarea
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                placeholder="You build the interface: layout, components, styling, and accessibility. Finish the whole task, then report what you changed."
                rows={6}
              />
            </Field>

            <Field label="What it can do (one line the orchestrator reads)">
              <Input
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="You research a question, weigh the sources, and answer with citations."
                className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
              />
            </Field>

            <div>
              <span className="mb-2 block text-sm text-white/60">Skills</span>
              <div className="overflow-hidden rounded-xl border border-white/10 bg-white/5">
                {skillsShown.length === 0 ? (
                  <p className="px-3 py-3 text-xs leading-relaxed text-white/35">
                    No skills installed. Add a folder with a SKILL.md inside to{" "}
                    <span className="font-mono text-white/50">
                      %USERPROFILE%\.calabrass\skills\
                    </span>
                    and reload the page.
                  </p>
                ) : (
                  <div className="scrollbar-thin flex max-h-44 flex-col overflow-y-auto">
                    {skillsShown.map((skill) => {
                      const checked = skills.includes(skill.name)
                      return (
                        <button
                          key={skill.name}
                          type="button"
                          onClick={() => toggleSkill(skill.name)}
                          aria-pressed={checked}
                          className={cn(
                            "flex items-start gap-3 px-3 py-2.5 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-white/20",
                            checked ? "bg-white/[0.06]" : "hover:bg-white/[0.03]"
                          )}
                        >
                          <span
                            className={cn(
                              "mt-0.5 grid size-4 shrink-0 place-items-center rounded border transition-colors",
                              checked
                                ? "border-[#9D79FF] bg-[#9D79FF] text-black"
                                : "border-white/20 bg-transparent"
                            )}
                          >
                            {checked ? (
                              <HugeiconsIcon
                                icon={Tick02Icon}
                                strokeWidth={3}
                                className="size-3"
                              />
                            ) : null}
                          </span>
                          <span className="min-w-0">
                            <span className="block text-xs font-medium text-white/80">
                              {skill.name}
                            </span>
                            {skill.description ? (
                              <span className="mt-0.5 block text-[11px] leading-relaxed text-white/40">
                                {skill.description}
                              </span>
                            ) : null}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
              <p className="mt-1.5 text-[11px] leading-relaxed text-white/30">
                A worker must read each selected skill with its read_skill tool
                before it starts work.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <Field label="Default model">
                <Picker
                  label="Default model"
                  value={model}
                  onChange={setModel}
                  options={modelOptions}
                  placeholder="Choose a model"
                />
              </Field>
              <Field label="Alternative model">
                <Picker
                  label="Alternative model"
                  value={altModel}
                  onChange={setAltModel}
                  options={altOptions}
                  placeholder="No fallback"
                />
              </Field>
            </div>
          </div>

          <DialogFooter className="sm:justify-between">
            {agent && !agent.builtin ? (
              <Button
                type="button"
                variant="destructive"
                onClick={() => void remove()}
                disabled={saving}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={1.8} />
                Delete
              </Button>
            ) : (
              <span />
            )}
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button
                type="button"
                variant="ghost"
                onClick={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={saving || !ready}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
