package app.modules

import scala.scalajs.js.annotation._
import scala.scalajs.js

@js.native
@JSGlobal
object Nostr extends js.Object {
  def getPublicKey(text: String): String = js.native
}
