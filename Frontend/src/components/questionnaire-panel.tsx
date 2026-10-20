import { useMemo, type FormEvent } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { MessageQuestionIcon } from "@hugeicons/core-free-icons"

import {
  Questionnaire,
  QuestionnaireActions,
  QuestionnaireChoice,
  QuestionnaireChoices,
  QuestionnaireError,
  QuestionnaireInput,
  QuestionnaireItem,
  QuestionnaireNext,
  QuestionnairePrevious,
  QuestionnaireSkip,
  QuestionnaireSubmit,
  QuestionnaireTitle,
} from "@/components/ui/questionnaire"
import type { AskQuestion } from "@/lib/questionnaire"

/**
 * The form the model's questions are shown in. It sits above the composer,
 * because the answers go back to the model as the next message and the
 * composer is where a message is written.
 */
export function QuestionnairePanel({
  questions,
  onSubmit,
  disabled = false,
}: {
  questions: AskQuestion[]
  onSubmit: (answers: Record<string, string>) => void
  /** The turn is still running, so the answers cannot be sent yet. */
  disabled?: boolean
}) {
  // The form's own bookkeeping: one item per question, named by its place in
  // the list, with the choices the model offered.
  const items = useMemo(
    () =>
      questions.map((question, index) => ({
        name: `q${index}`,
        required: question.options.length > 0,
        choices:
          question.options.length > 0
            ? question.options.map((option) => ({ value: option }))
            : undefined,
      })),
    [questions]
  )

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const answers: Record<string, string> = {}
    questions.forEach((_question, index) => {
      const values = data
        .getAll(`q${index}`)
        .map((value) => String(value))
        .filter((value) => value.trim() !== "")
      answers[String(index)] = values.join(", ")
    })
    onSubmit(answers)
  }

  return (
    <div className="mx-auto w-full max-w-[780px] px-4">
      {/* The card sits on the composer: rounded at the top, square and open at
          the bottom, so the two read as one surface. */}
      <div className="rounded-t-2xl border border-b-0 border-white/10 bg-white/5 p-4 backdrop-blur-xl">
        <div className="mb-4 flex items-center gap-2">
          <HugeiconsIcon
            icon={MessageQuestionIcon}
            strokeWidth={1.8}
            className="size-4 shrink-0 text-white/40"
          />
          <p className="text-sm font-medium text-white/90">
            The assistant needs a few answers before it goes on
          </p>
        </div>

        <Questionnaire items={items} onSubmit={handleSubmit} className="gap-4">
          {questions.map((question, index) => (
            <QuestionnaireItem
              key={`q${index}`}
              name={`q${index}`}
              required={question.options.length > 0}
              multiple={question.multiple}
            >
              <QuestionnaireTitle className="text-sm font-medium text-white/90">
                {question.question}
              </QuestionnaireTitle>

              {question.options.length > 0 ? (
                <QuestionnaireChoices className="gap-2">
                  {question.options.map((option) => (
                    <QuestionnaireChoice
                      key={option}
                      value={option}
                      className="min-h-9 border-white/10 bg-white/5 text-white/80 hover:bg-white/10 data-checked:border-primary/50 data-checked:bg-primary/15"
                    >
                      {option}
                    </QuestionnaireChoice>
                  ))}
                </QuestionnaireChoices>
              ) : (
                <QuestionnaireInput
                  type="text"
                  placeholder="Your answer"
                  className="border-white/10 bg-white/5 text-white placeholder:text-white/30"
                />
              )}

              <QuestionnaireError className="text-white/50">
                {question.options.length > 0
                  ? "Choose one to carry on."
                  : "Type an answer, or skip this one."}
              </QuestionnaireError>
            </QuestionnaireItem>
          ))}

          <QuestionnaireActions>
            <QuestionnairePrevious className="text-white/70">
              Back
            </QuestionnairePrevious>
            <QuestionnaireSkip className="text-white/50">
              Skip
            </QuestionnaireSkip>
            <QuestionnaireNext>Next</QuestionnaireNext>
            <QuestionnaireSubmit disabled={disabled}>
              Send answers
            </QuestionnaireSubmit>
          </QuestionnaireActions>
        </Questionnaire>

        {disabled ? (
          <p className="mt-3 text-xs text-white/40">
            The assistant is still finishing its turn. The answers can be sent
            as soon as it stops.
          </p>
        ) : null}
      </div>
    </div>
  )
}
