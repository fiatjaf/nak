import cats.data.*
import cats.effect.*
import cats.effect.syntax.all.*
import cats.syntax.all.*
import fs2.concurrent.*
import fs2.dom.{Event => _, *}
import io.circe.parser.*
import io.circe.syntax.*
import calico.*
import calico.html.io.{*, given}
import calico.syntax.*
import scoin.*
import snow.*

import Utils.*
import Components.*

object Store {
  def apply(window: Window[IO]): Resource[IO, Store] = {
    val key = "nak-input"

    for {
      inputRef <- SignallingRef[IO].of("").toResource

      _ <- Resource.eval {
        OptionT(window.localStorage.getItem(key))
          .foreachF(inputRef.set(_))
      }

      _ <- window.localStorage
        .events(window)
        .foreach {
          case Storage.Event.Updated(`key`, _, value, _) =>
            inputRef.set(value)
          case _ => IO.unit
        }
        .compile
        .drain
        .background

      _ <- inputRef.discrete
        .foreach(input => IO.cede *> window.localStorage.setItem(key, input))
        .compile
        .drain
        .background
    } yield new Store(inputRef)
  }
}

case class Store(input: SignallingRef[IO, String])

object Main extends IOWebApp {
  def render: Resource[IO, HtmlDivElement[IO]] = Store(window).flatMap {
    store =>
      div(
        cls := "flex w-full h-full flex-col items-center justify-center",
        div(
          cls := "w-4/5",
          h1(cls := "px-1 py-2 text-center text-xl", "nostr army knife"),
          div(
            cls := "flex my-3",
            input(store),
            actions(store)
          ),
          result(store)
        )
      )
  }

  def actions(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "flex flex-col space-y-1 my-3",
      button(
        Styles.button,
        "format",
        onClick --> (_.foreach(_ =>
          store.input.update(original =>
            parse(original).toOption
              .map(_.printWith(jsonPrinter))
              .getOrElse(original)
          )
        ))
      ),
      button(
        Styles.button,
        "generate event",
        onClick --> (_.foreach(_ =>
          store.input.set(
            Event(
              kind = 1,
              content = "hello world"
            ).sign(PrivateKey(randomBytes32()))
              .asJson
              .printWith(jsonPrinter)
          )
        ))
      )
    )

  def input(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "w-full grow",
      div(
        cls := "w-full flex justify-center",
        textArea.withSelf { self =>
          (
            cls := "w-full max-h-96 p-3 rounded",
            styleAttr := "min-height: 280px; font-family: monospace",
            spellCheck := false,
            placeholder := "paste something nostric",
            onInput --> (_.foreach(_ =>
              self.value.get.flatMap(store.input.set)
            )),
            value <-- store.input
          )
        }
      )
    )

  def result(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "w-full flex my-5",
      store.input.map { input =>
        if input.trim() == "" then div("")
        else
          Parser.parseInput(input) match {
            case Left(msg) => div(msg)
            case Right(event: Event) =>
              renderEvent(event)
            case Right(pp: ProfilePointer) => renderProfilePointer(pp)
            case Right(evp: EventPointer)  => renderEventPointer(evp)
            case Right(sk: PrivateKey) =>
              renderProfilePointer(
                ProfilePointer(pubkey = sk.publicKey.xonly),
                Some(sk)
              )
            case Right(addr: AddressPointer) => renderAddressPointer(addr)
          }

      }
    )
}
