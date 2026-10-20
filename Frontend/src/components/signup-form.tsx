import { useState } from "react"
import { cn } from "cn"
import { Link, useNavigate } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { signUp } from "@/lib/auth"
import { notifyError, notifySuccess } from "@/lib/toast"

/**
 * The sign-up form. It creates a real account in the local database: the
 * backend hashes the password and returns a session, so the person lands in
 * the app already signed in.
 *
 * The confirmation is checked here because the server only ever sees one
 * password; everything else (a valid email, a long enough password, an email
 * that is not already taken) is the server's call and comes back as its
 * message.
 */
export function SignupForm({
  className,
  ...props
}: React.ComponentProps<"form">) {
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirmation, setConfirmation] = useState("")
  const [pending, setPending] = useState(false)

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()

    if (password.length < 8) {
      notifyError(
        "Use at least 8 characters for your password.",
        "The password is too short"
      )
      return
    }
    if (password !== confirmation) {
      notifyError("Those passwords do not match.", "The passwords do not match")
      return
    }

    setPending(true)
    const result = await signUp(name, email, password)
    setPending(false)

    if (!result.ok) {
      notifyError(result.error, "Could not create the account")
      return
    }
    notifySuccess("Your account is ready.", "Welcome to Calabrass")
    void navigate({ to: "/" })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className={cn("flex flex-col gap-6", className)}
      aria-busy={pending}
      {...props}
    >
      <FieldGroup>
        <div className="flex flex-col items-center gap-1 text-center">
          <h1 className="text-2xl font-bold">Create your account</h1>
          <p className="text-sm text-balance text-muted-foreground">
            Enter your details below to get started
          </p>
        </div>

        <Field>
          <FieldLabel htmlFor="name">Name</FieldLabel>
          <Input
            id="name"
            type="text"
            autoComplete="name"
            placeholder="Your name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="email">Email</FieldLabel>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            placeholder="m@example.com"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="password">Password</FieldLabel>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </Field>

        <Field>
          <FieldLabel htmlFor="confirm-password">Confirm password</FieldLabel>
          <Input
            id="confirm-password"
            type="password"
            autoComplete="new-password"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            required
          />
        </Field>

        <Field>
          <Button type="submit" disabled={pending}>
            {pending ? "Creating account…" : "Create account"}
          </Button>
        </Field>

        <FieldDescription className="text-center">
          Already have an account?{" "}
          <Link to="/login" className="underline underline-offset-4">
            Login
          </Link>
        </FieldDescription>
      </FieldGroup>
    </form>
  )
}
