import type { ChatMessage, TracePart } from "@/types/chat"

/**
 * The questions the /ask command shows: one of each kind, so the form can be
 * tried without a model and without a provider key.
 */
export const ASK_TEST_QUESTIONS: AskQuestion[] = [
  {
    question: "Which one should I take?",
    options: ["The first", "The second", "The third"],
    multiple: false,
  },
  {
    question: "Which of these apply?",
    options: ["This one", "That one", "The other one"],
    multiple: true,
  },
  {
    question: "What should I call it?",
    options: [],
    multiple: false,
  },
]

/** The tool the model asks its questions with. */
export const ASK_QUESTIONS_TOOL = "ask_questions"

/**
 * The line that opens the message carrying the answers back to the model. It
 * is what tells a questionnaire that has already been answered from one that
 * is still waiting.
 */
export const ANSWERS_MARKER = "Answers to your questions:"

/** One question the model asked, in the shape the form wants. */
export interface AskQuestion {
  question: string
  /** The choices it offered, if it offered any. */
  options: string[]
  /** Whether more than one choice may be picked. */
  multiple: boolean
}

/**
 * The questions a tool call carried. The model's arguments are raw JSON, and a
 * malformed one simply yields nothing rather than breaking the page.
 *
 * The names the tool describes are "question" and "options". Models also write
 * "description" for the question and "values" for the choices, and sometimes
 * send the list itself rather than an object with a list in it, so all of those
 * are read as well.
 */
export function parseAskQuestions(input: string | undefined): AskQuestion[] {
  if (!input || input.trim() === "") return []

  let parsed: unknown
  try {
    parsed = JSON.parse(input)
  } catch {
    return []
  }
  // Some providers hand the arguments over as a string of JSON rather than as
  // JSON, so the text is read once more before it is given up on.
  if (typeof parsed === "string") {
    try {
      parsed = JSON.parse(parsed)
    } catch {
      return []
    }
  }

  const raw = questionsOf(parsed)
  const questions: AskQuestion[] = []
  for (const entry of raw) {
    if (typeof entry === "string") {
      const question = entry.trim()
      if (question === "") continue
      questions.push({ question, options: [], multiple: false })
      continue
    }
    if (!entry || typeof entry !== "object") continue

    const asked = entry as {
      question?: unknown
      description?: unknown
      text?: unknown
      options?: unknown
      values?: unknown
      choices?: unknown
      multiple?: unknown
    }
    const question = firstText(asked.question, asked.description, asked.text)
    if (question === "") continue

    const options = firstList(asked.options, asked.values, asked.choices)

    questions.push({
      question,
      options,
      multiple: asked.multiple === true,
    })
  }
  return questions
}

/** The list of questions, wherever the call put it. */
function questionsOf(parsed: unknown): unknown[] {
  if (Array.isArray(parsed)) return parsed
  if (!parsed || typeof parsed !== "object") return []

  const call = parsed as { questions?: unknown; question?: unknown }
  const list = call.questions ?? call.question
  if (Array.isArray(list)) return list
  if (list && typeof list === "object") return [list]
  return []
}

/** The first of the values that carries something. */
function firstText(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim() !== "") return value.trim()
  }
  return ""
}

/** The first of the lists that carries something. */
function firstList(...values: unknown[]): string[] {
  for (const value of values) {
    if (!Array.isArray(value)) continue
    const kept = value
      .filter(
        (option): option is string =>
          typeof option === "string" && option.trim() !== ""
      )
      .map((option) => option.trim())
    if (kept.length > 0) return kept
  }
  return []
}

/** The tool call that asked the questions, in one message's trace. */
function findAskQuestionsCall(
  message: ChatMessage | undefined
): TracePart | undefined {
  const steps = message?.trace?.steps
  if (!steps) return undefined
  for (const step of steps) {
    for (const part of step.parts ?? []) {
      if (part.kind === "tool" && part.name === ASK_QUESTIONS_TOOL) {
        return part
      }
    }
  }
  return undefined
}

/**
 * The questions waiting to be answered, if the model has asked any and the
 * answers have not come back yet.
 *
 * The reply that is still streaming counts too, so the form appears as soon as
 * the model asks rather than when its turn ends.
 */
export function findPendingQuestionnaire(
  messages: ChatMessage[],
  inFlight?: ChatMessage
): AskQuestion[] | null {
  const all = inFlight ? [...messages, inFlight] : messages

  let askedAt = -1
  let call: TracePart | undefined
  for (let i = 0; i < all.length; i++) {
    const found = findAskQuestionsCall(all[i])
    if (found) {
      askedAt = i
      call = found
    }
  }
  if (!call) return null

  // A user message after the call that carries the answers closes it.
  const answered = all
    .slice(askedAt + 1)
    .some(
      (message) =>
        message.role === "user" &&
        message.content.trimStart().startsWith(ANSWERS_MARKER)
    )
  if (answered) return null

  const questions = parseAskQuestions(call.input)
  return questions.length > 0 ? questions : null
}

/**
 * The message that carries the answers back to the model. It repeats the
 * questions with the answers under them, so the model has the whole thing in
 * front of it whatever it remembers of the turn before.
 */
export function formatAnswers(
  questions: AskQuestion[],
  answers: Record<string, string>
): string {
  const lines = [ANSWERS_MARKER]
  questions.forEach((question, index) => {
    const answer = (answers[String(index)] ?? "").trim()
    lines.push(
      `${index + 1}. ${question.question}${answer ? ` → ${answer}` : " → (no answer)"}`
    )
  })
  return lines.join("\n")
}
