import cats.effect.*
import calico.*
import calico.html.io.{*, given}
import calico.syntax.*
import fs2.dom.*

object Main extends IOWebApp {
  def render: Resource[IO, HtmlDivElement[IO]] = div(
    cls := "flex w-full h-full flex-col items-center justify-center",
    h1(cls := "px-1 py-2 text-lg", "nostr army knife"),
    Input()
  )
}
