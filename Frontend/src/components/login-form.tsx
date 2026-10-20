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
import { signIn } from "@/lib/auth"
import { notifyError } from "@/lib/toast"

/**
 * The login page's form.
 *
 * It posts the email and password to the backend, which checks the stored
 * bcrypt hash and returns a session token. The one message the server gives
 * for both a wrong email and a wrong password is said in a toast. New people
 * follow the link to the sign-up page.
 */
export function LoginForm({
  className,
  ...props
}: React.ComponentProps<"form">) {
  const navigate = useNavigate()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [pending, setPending] = useState(false)

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPending(true)

    const result = await signIn(email, password)
    setPending(false)

    if (!result.ok) {
      notifyError(result.error, "Could not sign in")
      return
    }
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
          <h1 className="text-2xl font-bold">Login to your account</h1>
          <p className="text-sm text-balance text-muted-foreground">
            Enter your email below to login to your account
          </p>
        </div>

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
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </Field>

        <Field>
          <Button type="submit" disabled={pending}>
            {pending ? "Logging in…" : "Login"}
          </Button>
        </Field>

        <FieldDescription className="text-center">
          Don&apos;t have an account?{" "}
          <Link to="/signup" className="underline underline-offset-4">
            Sign up
          </Link>
        </FieldDescription>
      </FieldGroup>
    </form>
  )
}
