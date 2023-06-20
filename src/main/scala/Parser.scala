import scala.util.Try
import io.circe.parser.*
import cats.syntax.all.*
import scodec.bits.ByteVector
import scoin.*
import snow.*

type Result = Either[
  String,
  Event | PrivateKey | AddressPointer | EventPointer | ProfilePointer |
    ByteVector32
]

object Parser {
  val additions = raw" *\+ *".r

  def parseInput(input: String): Result =
    if input == "" then Left("")
    else
      ByteVector
        .fromHex(input)
        .flatMap(b => Try(Right(ByteVector32(b))).toOption)
        .getOrElse(
          NIP19.decode(input) match {
            case Right(pp: ProfilePointer)             => Right(pp)
            case Right(evp: EventPointer)              => Right(evp)
            case Right(sk: PrivateKey)                 => Right(sk)
            case Right(addr: AddressPointer)           => Right(addr)
            case Left(_) if input.split(":").size == 3 =>
              // parse "a" tag format, nip 33
              val spl = input.split(":")
              (
                spl(0).toIntOption,
                ByteVector.fromHex(spl(1)),
                Some(spl(2))
              ).mapN((kind, author, identifier) =>
                AddressPointer(
                  identifier,
                  kind,
                  scoin.XOnlyPublicKey(ByteVector32(author)),
                  relays = List.empty
                )
              ).toRight("couldn't parse as a nip33 'a' tag")
            case Left(_) =>
              // parse event json
              parse(input) match {
                case Left(err: io.circe.ParsingFailure) =>
                  Left("not valid JSON or NIP-19 code")
                case Right(json) =>
                  json
                    .as[Event]
                    .leftMap { err =>
                      err.pathToRootString match {
                        case None       => s"decoding ${err.pathToRootString}"
                        case Some(path) => s"field $path is missing or wrong"
                      }
                    }
              }
          }
        )
}
