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

object Components {
  def renderEventPointer(
      evp: snow.EventPointer
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      entry("event id (hex)", evp.id),
      if evp.relays.size > 0 then
        Some(entry("relay hints", evp.relays.reduce((a, b) => s"$a, $b")))
      else None,
      evp.author.map { pk =>
        entry("author hint (pubkey hex)", pk.value.toHex)
      }
    )

  def renderProfilePointer(
      pp: snow.ProfilePointer,
      sk: Option[PrivateKey] = None
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      sk.map { k => entry("private key (hex)", k.value.toHex) },
      entry("public key (hex)", pp.pubkey.value.toHex),
      if pp.relays.size > 0 then
        Some(entry("relay hints", pp.relays.reduce((a, b) => s"$a, $b")))
      else None
    )

  def renderAddressPointer(
      addr: snow.AddressPointer
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      entry("author (pubkey hex)", addr.author.value.toHex),
      entry("identifier", addr.d),
      entry("kind", addr.kind.toString),
      if addr.relays.size > 0 then
        Some(entry("relay hints", addr.relays.reduce((a, b) => s"$a, $b")))
      else None
    )

  def renderEvent(event: Event): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      List(("pubkey", event.pubkey), ("id", event.id), ("sig", event.sig))
        .filter((_, v) => v.isEmpty)
        .map { (label, _) => entry("property missing", label) },
      entry("serialized event", event.serialized),
      entry("implied event id", event.hash.toHex),
      entry(
        "does the implied event id match the given event id?",
        event.id == Some(event.hash.toHex) match {
          case true => "yes"; case false => "no"
        }
      ),
      entry(
        "is signature valid?",
        event.isValid match {
          case true => "yes"; case false => "no"
        }
      )
    )

  private def entry(
      key: String,
      value: String
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      span(cls := "font-bold", key + " "),
      span(Styles.mono, value)
    )
}
