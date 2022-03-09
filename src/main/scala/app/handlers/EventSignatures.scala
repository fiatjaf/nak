package app.handlers

import scala.annotation.{nowarn, tailrec}
import scala.concurrent.ExecutionContext.Implicits.global
import scala.concurrent.Future
import scala.util.{Failure, Success}
import scala.scalajs.js
import slinky.core.FunctionalComponent
import slinky.web.html._
import slinky.core.facade.Hooks._
import slinky.core.facade.Fragment
import io.circe.{Json, HCursor}
import io.circe.parser.{parse}

import app.modules.Nostr
import app.handlers.{Handler}
import app.components.{Item}

object EventSignatures extends Handler {
  val keymatcher = "^[a-f0-9]{64}$".r

  def badProperties(c: HCursor): Seq[String] = Seq(
    (
      c.get[Double]("kind").getOrElse[Double](-1) >= 0 match {
        case true  => None;
        case false => Some("kind")
      }
    ),
    (
      keymatcher.matches(
        c.get[String]("pubkey").getOrElse("").toLowerCase()
      ) match {
        case true  => None;
        case false => Some("pubkey")
      }
    ),
    (
      c.get[String]("content").exists((_) => true) match {
        case true  => None;
        case false => Some("content")
      }
    ),
    (
      c
        .get[List[List[String]]]("tags")
        .exists((_) => true) match {
        case true  => None;
        case false => Some("tags")
      }
    )
  )
    .filter(res => res.isDefined)
    .map(res => res.get)

  override def handles(value: String): Boolean = parse(value) match {
    case Left(_) => false
    case Right(json) => {
      badProperties(json.hcursor).length < 4
    }
  }

  type MaybeItem = Future[
    Either[slinky.core.TagMod[Nothing], slinky.core.TagMod[Nothing]]
  ]

  @nowarn("cat=other")
  def itemWrongProperties(evtj: String): MaybeItem = Future {
    val c = parse(evtj).toOption.get.hcursor
    val bad = badProperties(c)

    if (bad.length > 0) {
      Left(
        Item.component(
          Item.props(
            "event missing or wrong properties",
            "",
            bad.mkString(", ")
          )
        )
      )
    } else {
      Right(div())
    }
  }

  @nowarn("cat=other")
  def itemSerializedEvent(evtj: String): MaybeItem = Future {
    val event: js.Dynamic = js.JSON.parse(evtj)

    Right(
      Item.component(
        Item.props(
          "serialized event",
          "according to nip-01 signature algorithm",
          Nostr.serializeEvent(event)
        )
      )
    )
  }

  @nowarn("cat=other")
  def itemEventId(evtj: String): MaybeItem = Future {
    val event: js.Dynamic = js.JSON.parse(evtj)

    Right(
      Item.component(
        Item.props(
          "event id",
          "sha256 hash of serialized event",
          Nostr.getEventHash(event)
        )
      )
    )
  }

  @nowarn("cat=other")
  def itemEventIdMatches(evtj: String): MaybeItem = Future {
    val c = parse(evtj).toOption.get.hcursor
    val event: js.Dynamic = js.JSON.parse(evtj)

    def render(result: Boolean) = Item.component(
      Item.props(
        "does the implied event id match the given event id?",
        "if not, that means you're hashing the event uncorrectly",
        f"${result match {
            case true => "yes"; case false => "no"
          }}"
      )
    )

    val hash = Nostr.getEventHash(event)

    c.get[String]("id") match {
      case Right(id) if id == hash => Right(render(true))
      case _                       => Left(render(false))
    }
  }

  @nowarn("cat=other")
  def itemSignatureValid(evtj: String): MaybeItem = {
    val event: js.Dynamic = js.JSON.parse(evtj)

    def render(result: Boolean) = Item.component(
      Item.props(
        "is signature valid?",
        "",
        f"${result match {
            case true => "yes"; case false => "no"
          }}"
      )
    )

    Nostr.verifySignature(event).toFuture map {
      case true  => Right(render(true))
      case false => Left(render(false))
    }
  }

  val protoElements = List[(String) => MaybeItem](
    itemWrongProperties,
    itemSerializedEvent,
    itemEventId,
    itemEventIdMatches,
    itemSignatureValid
  )

  @nowarn("cat=other")
  override val component = FunctionalComponent[String] { props =>
    val (elements, setElements) =
      useState(Seq.empty[slinky.core.TagMod[Nothing]])

    useEffect(
      () => {
        def runAndUnwrapUntilFirstLeft(
            remaining: List[String => Future[
              Either[slinky.core.TagMod[Nothing], slinky.core.TagMod[Nothing]]
            ]],
            acc: List[slinky.core.TagMod[Nothing]]
        ): Future[List[slinky.core.TagMod[Nothing]]] = remaining match {
          case fn :: tail => {
            fn(props) flatMap {
              {
                case Left(el)  => runAndUnwrapUntilFirstLeft(Nil, el :: acc)
                case Right(el) => runAndUnwrapUntilFirstLeft(tail, el :: acc)
              }
            }
          }
          case Nil => Future { acc.reverse }
        }

        runAndUnwrapUntilFirstLeft(protoElements, List()) onComplete {
          case Success(x) => setElements(x)
          case Failure(err) =>
            println(f"failed to run through elements: ${err}")
        }

        () => {}
      },
      Seq(props)
    )

    Fragment(elements: _*)
  }
}
