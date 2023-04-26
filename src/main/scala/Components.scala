import cats.data.{Store => *, *}
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
import scodec.bits.ByteVector
import scoin.*
import snow.*

import Utils.*

object Components {
  def render32Bytes(bytes32: ByteVector32): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      entry("canonical hex", bytes32.toHex),
      "if this is a public key:",
      div(
        cls := "mt-2 pl-2 mb-2",
        entry(
          "npub",
          NIP19.encode(XOnlyPublicKey(bytes32))
        ),
        nip19_21(
          "nprofile",
          NIP19.encode(ProfilePointer(XOnlyPublicKey(bytes32)))
        )
      ),
      "if this is a private key:",
      div(
        cls := "pl-2 mb-2",
        entry(
          "nsec",
          NIP19.encode(PrivateKey(bytes32))
        ),
        entry(
          "npub",
          NIP19.encode(XOnlyPublicKey(bytes32))
        ),
        nip19_21(
          "nprofile",
          NIP19.encode(ProfilePointer(XOnlyPublicKey(bytes32)))
        )
      ),
      "if this is an event id:",
      div(
        cls := "pl-2 mb-2",
        nip19_21(
          "nevent",
          NIP19.encode(EventPointer(bytes32.toHex))
        )
      ),
      div(
        cls := "pl-2 mb-2",
        entry(
          "note",
          NIP19.encode(bytes32)
        )
      )
    )

  def renderEventPointer(
      store: Store,
      evp: snow.EventPointer
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      entry("event id (hex)", evp.id),
      relayHints(store, evp.relays),
      evp.author.map { pk =>
        entry("author hint (pubkey hex)", pk.value.toHex)
      },
      nip19_21("nevent", NIP19.encode(evp)),
      entry("note", NIP19.encode(ByteVector32.fromValidHex(evp.id)))
    )

  def renderProfilePointer(
      store: Store,
      pp: snow.ProfilePointer,
      sk: Option[PrivateKey] = None
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      sk.map { k => entry("private key (hex)", k.value.toHex) },
      sk.map { k => entry("nsec", NIP19.encode(k)) },
      entry("public key (hex)", pp.pubkey.value.toHex),
      relayHints(
        store,
        pp.relays,
        dynamic = if sk.isDefined then false else true
      ),
      entry("npub", NIP19.encode(pp.pubkey)),
      nip19_21("nprofile", NIP19.encode(pp))
    )

  def renderAddressPointer(
      store: Store,
      addr: snow.AddressPointer
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      entry("author (pubkey hex)", addr.author.value.toHex),
      entry("identifier", addr.d),
      entry("kind", addr.kind.toString),
      relayHints(store, addr.relays),
      nip19_21("naddr", NIP19.encode(addr))
    )

  def renderEvent(
      store: Store,
      event: Event
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "text-md",
      if event.pubkey.isEmpty then
        Some(
          div(
            cls := "flex items-center",
            entry("missing", "pubkey"),
            button(
              Styles.buttonSmall,
              "fill with a debugging key",
              onClick --> (_.foreach { _ =>
                store.input.set(
                  event
                    .copy(pubkey = Some(keyOne.publicKey.xonly))
                    .asJson
                    .printWith(jsonPrinter)
                )
              })
            )
          )
        )
      else None,
      if event.id.isEmpty then
        Some(
          div(
            cls := "flex items-center",
            entry("missing", "id"),
            if event.pubkey.isDefined then
              Some(
                button(
                  Styles.buttonSmall,
                  "fill id",
                  onClick --> (_.foreach(_ =>
                    store.input.set(
                      event
                        .copy(id = Some(event.hash.toHex))
                        .asJson
                        .printWith(jsonPrinter)
                    )
                  ))
                )
              )
            else None
          )
        )
      else None,
      if event.sig.isEmpty then
        Some(
          div(
            cls := "flex items-center",
            entry("missing", "sig"),
            if event.id.isDefined && event.pubkey == Some(
                keyOne.publicKey.xonly
              )
            then
              Some(
                button(
                  Styles.buttonSmall,
                  "sign",
                  onClick --> (_.foreach(_ =>
                    store.input.set(
                      event
                        .sign(keyOne)
                        .asJson
                        .printWith(jsonPrinter)
                    )
                  ))
                )
              )
            else None
          )
        )
      else None,
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
      ),
      event.id.map(id =>
        nip19_21(
          "nevent",
          NIP19.encode(EventPointer(id, author = event.pubkey))
        )
      ),
      event.id.map(id =>
        entry(
          "note",
          NIP19.encode(ByteVector32.fromValidHex(id))
        )
      )
    )

  private def entry(
      key: String,
      value: String
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "flex items-center space-x-3",
      span(cls := "font-bold", key + " "),
      span(Styles.mono, cls := "max-w-xl", value)
    )

  private def nip19_21(
      key: String,
      code: String
  ): Resource[IO, HtmlDivElement[IO]] =
    div(
      span(cls := "font-bold", key + " "),
      span(Styles.mono, cls := "break-all", code),
      a(
        href := "nostr:" + code,
        external
      )
    )

  private def relayHints(
      store: Store,
      relays: List[String],
      dynamic: Boolean = true
  ): Resource[IO, HtmlDivElement[IO]] =
    if !dynamic && relays.isEmpty then div("")
    else
      SignallingRef[IO].of(false).toResource.flatMap { active =>
        val value =
          if relays.size > 0 then relays.reduce((a, b) => s"$a, $b") else ""

        div(
          cls := "flex items-center space-x-3",
          span(cls := "font-bold", "relay hints "),
          span(Styles.mono, cls := "max-w-xl", value),
          active.map {
            case true =>
              div(
                input.withSelf { self =>
                  (
                    onKeyPress --> (_.foreach(evt =>
                      evt.key match {
                        case "Enter" =>
                          self.value.get.flatMap(url =>
                            if url.startsWith("wss://") || url
                                .startsWith("ws://")
                            then
                              store.input.update(
                                _.trim() ++ " + " ++ url
                              ) >> active.set(false)
                            else IO.unit
                          )
                        case _ => IO.unit
                      }
                    ))
                  )
                }
              )
            case false if dynamic =>
              button(
                Styles.buttonSmall,
                "add relay hint",
                onClick --> (_.foreach(_ => active.set(true)))
              )
            case false => div("")
          }
        )
      }

  private val external = img(cls := "inline w-4 ml-2", src := "ext.svg")
}
